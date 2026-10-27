package provider

import (
	"context"
	"errors"
	"net/http"

	"github.com/chriha/lanes/internal/item"
)

// LinearTokenEnv is the env var the linear provider reads its API key from when token_env is not set.
const LinearTokenEnv = "LINEAR_API_KEY"

// Linear lists a Linear team's open issues as tickets through the Linear GraphQL API.
type Linear struct {
	Team     string       // team key, e.g. ENG
	TokenEnv string       // env var holding the API key; empty = LinearTokenEnv
	Endpoint string       // GraphQL endpoint; empty = the Linear API
	Client   *http.Client // nil = http.DefaultClient
}

func (l *Linear) Fetch(ctx context.Context) ([]item.Item, error) {
	return nil, errors.New("linear: not implemented")
}
