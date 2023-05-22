package config

const (
	PROMETHEUS  = "Prometheus"
	NODEMETRICS = "NodeMetrics"
)

// AnnotatorConfiguration holds configuration for a node annotator.
type AnnotatorConfiguration struct {
	// BindingHeapSize limits the size of Binding Heap, which stores the lastest
	// pod scheduled imformation.
	BindingHeapSize int32
	// ConcurrentSyncs specified the number of annotator controller workers.
	ConcurrentSyncs int32
	// PolicyConfigPath specified the path of Scheduler Policy File.
	PolicyConfigPath string
	// Mode
	Mode string
	// PrometheusAddr is the address of Prometheus Service.
	PrometheusAddr string
	// Node Statistics handler
	StatisticsPath string
	// Statistics Port
	StatisticsPort string
}
