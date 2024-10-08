package metrics

type StatEntry struct {
	Value    float64 `json:"metric_value"`
	Interval int     `json:"interval"`
}

type Stat struct {
	Entries []StatEntry
	PodName string
	PodIP   string
}

var emptyStat = Stat{}
var emptyStatEntry = StatEntry{}
