package out

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/mitchellh/mapstructure"
)

type OutRequest struct {
	Source Source    `json:"source"`
	Params OutParams `json:"params"`
}

type Source struct {
	URI        string        `json:"uri"`
	Branch     string        `json:"branch"`
	PrivateKey string        `json:"private_key" mapstructure:"private_key"`
	Pool       string        `json:"pool"`
	RetryDelay time.Duration `json:"retry_delay" mapstructure:"retry_delay"`
}

func (s *Source) UnmarshalJSON(b []byte) error {
	var inputData map[string]interface{}
	err := json.NewDecoder(bytes.NewReader(b)).Decode(&inputData)
	if err != nil {
		return err
	}

	decodeConfig := &mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
		Result:     s,
	}

	decoder, err := mapstructure.NewDecoder(decodeConfig)
	if err != nil {
		return err
	}

	err = decoder.Decode(inputData)
	if err != nil {
		return err
	}

	return nil
}

type OutParams struct {
	Release    string `json:"release"`
	Acquire    bool   `json:"acquire"`
	Add        string `json:"add"`
	AddClaimed string `json:"add_claimed"`
	Remove     string `json:"remove"`
	Claim      string `json:"claim"`
	Update     string `json:"update"`
}

func (request OutRequest) Validate() []string {
	var errorMessages []string

	if request.Source.URI == "" {
		errorMessages = append(errorMessages, "invalid payload (missing uri)")
	}

	if request.Source.Pool == "" {
		errorMessages = append(errorMessages, "invalid payload (missing pool)")
	}

	if request.Source.Branch == "" {
		errorMessages = append(errorMessages, "invalid payload (missing branch)")
	}

	if request.Params.Acquire == false &&
		request.Params.Release == "" &&
		request.Params.Add == "" &&
		request.Params.AddClaimed == "" &&
		request.Params.Remove == "" &&
		request.Params.Claim == "" &&
		request.Params.Update == "" {
		errorMessages = append(errorMessages, "invalid payload (missing acquire, release, remove, claim, add, or add_claimed)")
	}

	return errorMessages
}
