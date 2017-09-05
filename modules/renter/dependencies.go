package renter

type prodDependencies struct{}

func (prodDependencies) Disrupt(string) bool { return false }

type BlockRepairUpload struct {
	prodDependencies
}

func (BlockRepairUpload) disrupt(s string) bool {
	if s == "BlockRemoteRepair" {
		return true
	}
	if s == "NoChunkCaching" {
		return true
	}
	return false
}
