package probe

import (
	"hash/fnv"
	"strings"
	"time"
)

type probePrompt struct {
	Category        string
	Content         string
	MaxOutputTokens uint
}

var probePromptLibrary = []probePrompt{
	{
		Category:        PromptCategoryShort,
		Content:         "Reply with exactly: ok",
		MaxOutputTokens: 8,
	},
	{
		Category:        PromptCategoryZH,
		Content:         "请只回复两个字：正常",
		MaxOutputTokens: 8,
	},
	{
		Category:        PromptCategoryMedium,
		Content:         "Read this short status note and reply with exactly one word, ok. Status note: the gateway is checking whether a text-only model request can start streaming and finish normally.",
		MaxOutputTokens: 12,
	},
	{
		Category: PromptCategoryLong,
		Content: strings.Join([]string{
			"Summarize the following diagnostic text in exactly one short sentence.",
			"The service routes user requests across several upstream model channels.",
			"A health probe must verify that the selected channel can accept a normal text request, send the first byte quickly, and complete without upstream errors.",
			"This payload is intentionally text-only and low cost, but longer than the usual heartbeat prompt so duration scoring has a usable signal.",
		}, " "),
		MaxOutputTokens: 32,
	},
}

func selectProbePromptCategory(candidate ProbeCandidate) string {
	categories := NormalizePromptCategories(candidate.PromptCategories)
	if candidate.Channel != nil && len(candidate.Channel.GetModels()) > 0 {
		// Keep the hash stable for a channel/model group while still rotating over time.
		seed := strings.Join([]string{
			candidate.Model,
			candidate.Group,
			candidate.Key.CapabilityFingerprint,
			time.Now().UTC().Format("200601021504"),
		}, "\x00")
		return categories[hashProbePromptSeed(seed)%len(categories)]
	}
	seed := strings.Join([]string{
		candidate.Model,
		candidate.Group,
		time.Now().UTC().Format("200601021504"),
	}, "\x00")
	return categories[hashProbePromptSeed(seed)%len(categories)]
}

func probePromptForCategory(category string) probePrompt {
	category = strings.TrimSpace(category)
	for _, prompt := range probePromptLibrary {
		if prompt.Category == category {
			return prompt
		}
	}
	return probePromptLibrary[0]
}

func hashProbePromptSeed(seed string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return int(h.Sum32())
}
