package types

import "reflect"

type DenomMetadata struct {
	Base     string `mapstructure:"base_denom"`
	Display  string `mapstructure:"display_denom"`
	Exponent uint32 `mapstructure:"exponent"`
}

func NewDenomMetadata(base string, display string, exponent uint32) DenomMetadata {
	return DenomMetadata{
		Base:     base,
		Display:  display,
		Exponent: exponent,
	}
}

func (x DenomMetadata) IsStructureEmpty() bool {
	return reflect.DeepEqual(x, DenomMetadata{})
}
