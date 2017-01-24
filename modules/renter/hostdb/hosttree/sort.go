package hosttree

type hostEntries []hostEntry

func (he hostEntries) Len() int           { return len(he) }
func (he hostEntries) Less(i, j int) bool { return he[i].weight.Cmp(he[j].weight) < 0 }
func (he hostEntries) Swap(i, j int)      { he[i], he[j] = he[j], he[i] }
