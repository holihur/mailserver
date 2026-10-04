package health

import "testing"

func TestEvaluate(t *testing.T) {
	// 50 正常；85 警告；95 严重
	alerts := evaluate(Snapshot{CPU: 50, Memory: 85, Disk: 95})
	if len(alerts) != 2 {
		t.Fatalf("应产生 2 条告警，得到 %d: %+v", len(alerts), alerts)
	}
	byMetric := map[string]string{}
	for _, a := range alerts {
		byMetric[a.Metric] = a.Level
	}
	if byMetric["memory"] != "warn" {
		t.Fatalf("memory 应为 warn: %+v", alerts)
	}
	if byMetric["disk"] != "critical" {
		t.Fatalf("disk 应为 critical: %+v", alerts)
	}
	if _, ok := byMetric["cpu"]; ok {
		t.Fatalf("cpu 不应告警: %+v", alerts)
	}
	// 边界：正好 80 / 90
	a80 := evaluate(Snapshot{CPU: 80, Memory: 80, Disk: 80})
	if len(a80) != 3 {
		t.Fatalf("80%% 应全部 warn: %+v", a80)
	}
	a90 := evaluate(Snapshot{CPU: 90, Memory: 90, Disk: 90})
	if len(a90) != 3 {
		t.Fatalf("90%% 应全部 critical: %+v", a90)
	}
}

func TestCollectNoPanic(t *testing.T) {
	c := New("/")
	_ = c.Current()
	_ = c.readCPU()
	_, _, _ = readMemory()
	_, _, _ = readDisk("/")
	// 不存在的路径不应 panic
	_, _, _ = readDisk("/nonexistent-path-xyz")
}
