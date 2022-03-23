package types

type DenomNotFound struct{}

func (m *DenomNotFound) Error() string {
	return "No denom infos"
}
