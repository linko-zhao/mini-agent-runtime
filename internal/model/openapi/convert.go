package openapi

import (
	"encoding/json"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

/*
{
  "id": "chatcmpl-B9MBs8CjcvOU2jLn4n570S5qMJKcT",
  "object": "chat.completion",
  "created": 1741569952,
  "model": "gpt-6-astra",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I assist you today?",
        "refusal": null,
        "annotations": []
      },
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 19,
    "completion_tokens": 10,
    "total_tokens": 29,
    "prompt_tokens_details": {
      "cached_tokens": 0,
      "audio_tokens": 0
    },
    "completion_tokens_details": {
      "reasoning_tokens": 0,
      "audio_tokens": 0,
      "accepted_prediction_tokens": 0,
      "rejected_prediction_tokens": 0
    }
  },
  "service_tier": "default"
}
*/
type Message struct {
	Role string `json: "role"`
	Content string `json: "content"`
	Refusal string `json: "refusal"`
}

type Choice struct {
	Index int `json: "index"`
	Message model.Model `json: "message"`
	FinishReason model.FinishReason `json: "finish_reason`
}

type ChatCompletions struct {
	Choices	[]model.Message `json: "choices"`
	
}


