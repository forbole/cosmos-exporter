package types

type DenomMetadata struct {
	Base    string
	Display string
	Denoms  map[string]DenomUnit
}

type DenomUnit struct {
	Denom    string
	Exponent uint32
}

func NewDenomUnit(denom string, exponent uint32) DenomUnit {
	return DenomUnit{
		Denom:    denom,
		Exponent: exponent,
	}
}

func NewDenomMetadata(base string, display string, denoms map[string]DenomUnit) DenomMetadata {
	return DenomMetadata{
		Base:    base,
		Display: display,
		Denoms:  denoms,
	}
}
