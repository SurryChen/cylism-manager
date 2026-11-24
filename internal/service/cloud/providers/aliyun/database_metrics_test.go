package cloud

import (
	"testing"
)

func TestRDSMetricsRespectsCompositeFieldOrderAndMissingSamples(t *testing.T) {
	result, err := parseRDSMetrics([]byte(`{"PerformanceKeys":{"PerformanceKey":[{"Key":"MySQL_MemCpuUsage","Unit":"%","ValueFormat":"mem_usage&cpu_usage","Values":{"PerformanceValue":[{"Date":"2026-11-07T03:00:00Z","Value":"31.5&2"},{"Date":"2026-11-07T03:01Z","Value":"null&3"},{"Date":"invalid","Value":"0&0"},{"Date":"2026-11-07T03:02Z","Value":"NaN&Inf"}]}}]}}`), "rm-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Series) != 2 || result.Series[0].Label != "内存使用率" || result.Series[1].Label != "CPU 使用率" {
		t.Fatalf("incorrect order: %+v", result)
	}
	if len(result.Series[0].Values) != 1 || len(result.Series[1].Values) != 2 || result.Series[0].Values[0].Value != 31.5 || result.Series[1].Values[1].Value != 3 {
		t.Fatalf("missing values invented or dropped: %+v", result.Series)
	}
}
func TestRDSMetricsEmptyAndInvalidResponses(t *testing.T) {
	if _, err := parseRDSMetrics([]byte(`not json`), "rm-test"); err == nil {
		t.Fatal("invalid response accepted")
	}
	result, err := parseRDSMetrics([]byte(`{"PerformanceKeys":{"PerformanceKey":[]}}`), "rm-test")
	if err != nil || result.Series == nil || len(result.Series) != 0 {
		t.Fatalf("empty metrics should remain empty: %+v %v", result, err)
	}
}

func TestRDSMetricsProviderFieldNamesAndNumericUnits(t *testing.T) {
	result, err := parseRDSMetrics([]byte(`{"PerformanceKeys":{"PerformanceKey":[{"Key":"MySQL_MemCpuUsage","Unit":"%","ValueFormat":"cpuusage&memusage"},{"Key":"MySQL_QPSTPS","Unit":"int&int","ValueFormat":"qps&tps"}]}}`), "rm-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Series) != 4 || result.Series[0].Label != "CPU 使用率" || result.Series[1].Label != "内存使用率" || result.Series[0].Unit != "%" {
		t.Fatalf("unexpected metric labels: %+v", result.Series)
	}
	if result.Series[2].Unit != "" || result.Series[3].Unit != "" {
		t.Fatalf("numeric types must not be displayed as units: %+v", result.Series)
	}
}
