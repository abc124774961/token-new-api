package proxy

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
)

type ConvertRequestInput struct {
	ProviderProfile string
	ProxyMode       string
	RequestModel    string
	UpstreamModel   string
	RequestBody     []byte
}

type ConvertRequestResult struct {
	ProviderProfile string
	ProxyMode       string
	UpstreamPath    string
	UpstreamBody    []byte
	UpstreamRequest any
}

type ConvertResponseInput struct {
	ProviderProfile string
	ProxyMode       string
	RequestModel    string
	UpstreamModel   string
	ResponseBody    []byte
}

type ConvertStreamInput struct {
	ProviderProfile string
	ProxyMode       string
	RequestModel    string
	UpstreamModel   string
	UpstreamEvents  []string
}

type ConvertResponseResult struct {
	ProviderProfile  string
	ProxyMode        string
	DownstreamBody   []byte
	DownstreamResult any
}

type ConvertStreamResult struct {
	ProviderProfile  string
	ProxyMode        string
	DownstreamEvents []string
	OutputText       string
	ReasoningText    string
	ToolName         string
	ToolArguments    string
	Usage            *dto.Usage
}

type Engine struct {
	registry provider.ProviderRegistry
}

type ProxyEngine interface {
	ConvertRequest(input ConvertRequestInput) (*ConvertRequestResult, error)
	ConvertResponse(input ConvertResponseInput) (*ConvertResponseResult, error)
	ConvertStream(input ConvertStreamInput) (*ConvertStreamResult, error)
	NewStreamConverter(input ConvertStreamInput) (StreamConverter, error)
}

type StreamConverter interface {
	Accept(upstreamEvent string) (*ConvertStreamResult, error)
	Finish() (*ConvertStreamResult, error)
}

type ResponsesToChatConverter interface {
	ConvertRequest(req *dto.OpenAIResponsesRequest, upstreamModel string) (*dto.GeneralOpenAIRequest, error)
	ConvertResponse(resp *dto.OpenAITextResponse, requestModel string) (*dto.OpenAIResponsesResponse, error)
	ConvertStream(events []string, requestModel string) (*ConvertStreamResult, error)
}
