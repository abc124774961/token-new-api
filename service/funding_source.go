package service

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包 or 订阅）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet" 或 "subscription"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

type WalletFunding struct {
	userId   int
	consumed int // 实际预扣的用户额度
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	if err := model.DecreaseUserQuota(w.userId, amount, false); err != nil {
		return err
	}
	w.consumed = amount
	return nil
}

func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		if err := model.DecreaseUserQuota(w.userId, delta, false); err != nil {
			return err
		}
		w.consumed += delta
		return nil
	}
	refund := -delta
	if err := model.IncreaseUserQuota(w.userId, refund, false); err != nil {
		return err
	}
	w.consumed -= refund
	if w.consumed < 0 {
		w.consumed = 0
	}
	return nil
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 {
		return nil
	}
	// IncreaseUserQuota 是 quota += N 的非幂等操作，不能重试，否则会多退额度。
	// 订阅的 RefundSubscriptionPreConsume 有 requestId 幂等保护所以可以重试。
	return model.IncreaseUserQuota(w.userId, w.consumed, false)
}

// ---------------------------------------------------------------------------
// SplitFunding — 订阅优先 + 钱包补足的混合资金来源
// ---------------------------------------------------------------------------

type SplitFunding struct {
	subscription *SubscriptionFunding
	wallet       *WalletFunding
	subPostDelta int64
}

func (s *SplitFunding) Source() string { return BillingSourceSubscriptionWallet }

func (s *SplitFunding) PreConsume(_ int) error {
	if s == nil || s.subscription == nil || s.wallet == nil {
		return nil
	}
	if err := s.subscription.PreConsumePartial(); err != nil {
		return err
	}
	remaining := int(s.subscription.amount - s.subscription.preConsumed)
	if remaining <= 0 {
		return nil
	}
	userQuota, err := model.GetUserQuota(s.wallet.userId, false)
	if err != nil {
		_ = s.subscription.Refund()
		return err
	}
	if userQuota < remaining {
		_ = s.subscription.Refund()
		return errors.New("wallet quota insufficient")
	}
	if err := s.wallet.PreConsume(remaining); err != nil {
		_ = s.subscription.Refund()
		return err
	}
	return nil
}

func (s *SplitFunding) Settle(delta int) error {
	if s == nil {
		return nil
	}
	if delta == 0 {
		return nil
	}
	if delta < 0 {
		refund := -delta
		if s.wallet != nil && s.wallet.consumed > 0 {
			walletRefund := refund
			if walletRefund > s.wallet.consumed {
				walletRefund = s.wallet.consumed
			}
			if walletRefund > 0 {
				if err := model.IncreaseUserQuota(s.wallet.userId, walletRefund, false); err != nil {
					return err
				}
				s.wallet.consumed -= walletRefund
				refund -= walletRefund
			}
		}
		if refund > 0 && s.subscription != nil && s.subscription.subscriptionId > 0 {
			if err := model.PostConsumeUserSubscriptionDelta(s.subscription.subscriptionId, -int64(refund)); err != nil {
				return err
			}
			s.subPostDelta -= int64(refund)
		}
		return nil
	}

	remaining := delta
	var subConsumed int64
	if s.subscription != nil && s.subscription.subscriptionId > 0 {
		consumed, err := model.PostConsumeUserSubscriptionDeltaAvailable(s.subscription.subscriptionId, int64(remaining))
		if err != nil {
			return err
		}
		subConsumed = consumed
		s.subPostDelta += subConsumed
		remaining -= int(subConsumed)
	}
	if remaining > 0 && s.wallet != nil {
		if err := model.DecreaseUserQuota(s.wallet.userId, remaining, false); err != nil {
			if subConsumed > 0 && s.subscription != nil && s.subscription.subscriptionId > 0 {
				_ = model.PostConsumeUserSubscriptionDelta(s.subscription.subscriptionId, -subConsumed)
				s.subPostDelta -= subConsumed
				if s.subPostDelta < 0 {
					s.subPostDelta = 0
				}
			}
			return err
		}
		s.wallet.consumed += remaining
	}
	return nil
}

func (s *SplitFunding) Refund() error {
	if s == nil {
		return nil
	}
	var firstErr error
	if s.subscription != nil {
		firstErr = s.subscription.Refund()
		if s.subPostDelta > 0 && s.subscription.subscriptionId > 0 {
			if err := model.PostConsumeUserSubscriptionDelta(s.subscription.subscriptionId, -s.subPostDelta); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if s.wallet != nil {
		if err := s.wallet.Refund(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

type SubscriptionFunding struct {
	requestId      string
	userId         int
	modelName      string
	amount         int64 // 预扣的订阅额度（subConsume）
	subscriptionId int
	preConsumed    int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
}

func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (s *SubscriptionFunding) PreConsume(_ int) error {
	// amount 参数被忽略，使用内部 s.amount（已在构造时根据 preConsumedQuota 计算）
	res, err := model.PreConsumeUserSubscription(s.requestId, s.userId, s.modelName, 0, s.amount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

func (s *SubscriptionFunding) PreConsumePartial() error {
	res, err := model.PreConsumeUserSubscriptionPartial(s.requestId, s.userId, s.modelName, 0, s.amount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return model.PostConsumeUserSubscriptionDelta(s.subscriptionId, int64(delta))
}

func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}
