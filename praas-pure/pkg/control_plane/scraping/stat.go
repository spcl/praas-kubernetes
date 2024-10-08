package scraping

type StatEntry struct {
	Value    float64 `json:"metric_value"`
	Interval int     `json:"interval"`
}
