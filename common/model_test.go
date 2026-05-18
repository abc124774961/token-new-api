package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGPTImage2EndpointTypes(t *testing.T) {
	for _, model := range []string{"gpt-image-1", "gpt-image-2", "gpt-image-3"} {
		if !IsImageGenerationModel(model) {
			t.Fatalf("expected %s to be detected as image generation model", model)
		}
	}

	endpoints := GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-image-2")
	if len(endpoints) < 3 {
		t.Fatalf("expected image and OpenAI endpoints, got %#v", endpoints)
	}
	if endpoints[0] != constant.EndpointTypeImageGeneration {
		t.Fatalf("expected first endpoint to be image-generation, got %q", endpoints[0])
	}
	if endpoints[1] != constant.EndpointTypeImageEdit {
		t.Fatalf("expected second endpoint to be image-edit, got %q", endpoints[1])
	}
	if endpoints[2] != constant.EndpointTypeOpenAI {
		t.Fatalf("expected third endpoint to be openai, got %q", endpoints[2])
	}
}
