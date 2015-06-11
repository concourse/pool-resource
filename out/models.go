package out

import "time"

type Source struct {
	URI        string        `json:"uri"`
	Branch     string        `json:"branch"`
	PrivateKey string        `json:"private_key"`
	Pool       string        `json:"pool"`
	RetryDelay time.Duration `json:"retry_delay"`
}

type Version struct {
	Ref string `json:"ref"`
}

type OutParams struct {
	Release string `json:"release"`
	Acquire bool   `json:"acquire"`
}

type OutRequest struct {
	Source Source    `json:"source"`
	Params OutParams `json:"params"`
}

type OutResponse struct {
	Version  Version        `json:"version"`
	Metadata []MetadataPair `json:"metadata"`
}

type MetadataPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
