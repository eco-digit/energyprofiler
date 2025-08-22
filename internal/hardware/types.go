package hardware

type CPUInfo struct {
	Cores        int
	Threads      int
	Architecture string
}

type RAMInfo struct {
	TotalCapacity uint64
	Channels      int
	Speed         int
	TheoreticalBW float64
	Type          string
}
