package models

type ManualInput struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Default     *string
	Options     []string
}

type ManualInputs struct {
	Known   bool
	Enabled bool
	Ref     string
	Sha     string
	Inputs  []ManualInput
}
