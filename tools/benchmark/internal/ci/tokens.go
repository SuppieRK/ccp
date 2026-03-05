package ci

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

var (
	tokenizerOnce sync.Once
	tokenizer     *tiktoken.Tiktoken
)

func TokenizerID() string {
	return "tiktoken-go:gpt-4o"
}

func CountTokens(text string) int {
	tokenizerOnce.Do(func() {
		// Keep model choice stable for benchmark trend comparability.
		enc, err := tiktoken.EncodingForModel("gpt-4o")
		if err != nil {
			tokenizer = nil
			return
		}
		tokenizer = enc
	})
	if tokenizer == nil || text == "" {
		return 0
	}
	return len(tokenizer.Encode(text, nil, nil))
}
