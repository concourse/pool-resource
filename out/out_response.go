package out

type Version struct {
	Ref string `json:"ref"`
}

type OutResponse struct {
	Version  Version        `json:"version"`
	Metadata []MetadataPair `json:"metadata"`
}

type MetadataPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
