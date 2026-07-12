package database

import (
	"os"
	"testing"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/pkg/models"
)

func testDB(t *testing.T, name string) (*DB, string) {
	t.Helper()
	path := "/tmp/bandwidth-test-" + name + ".db"
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })

	db, err := Open(Config{
		Path:         path,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
		JournalMode:  "WAL",
		Synchronous:  "NORMAL",
		CacheSizeKB:  2000,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func TestOpenAndPing(t *testing.T) {
	db, _ := testDB(t, "ping")
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestUpsertAndListContainers(t *testing.T) {
	db, _ := testDB(t, "containers")

	c := &models.Container{
		ID:            "abc123test",
		Name:          "test-container",
		VethInterface: "vethTest0",
		State:         models.StateRunning,
		LimitRxMbps:   100,
		LimitTxMbps:   50,
		DailyQuotaGB:  500,
	}

	if err := db.UpsertContainer(c); err != nil {
		t.Fatalf("UpsertContainer: %v", err)
	}

	containers, err := db.ListContainers("")
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}

	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	got := containers[0]
	if got.Name != "test-container" {
		t.Errorf("expected name test-container, got %s", got.Name)
	}
	if got.LimitRxMbps != 100 {
		t.Errorf("expected rx 100, got %g", got.LimitRxMbps)
	}
}

func TestListByState(t *testing.T) {
	db, _ := testDB(t, "state")

	running := &models.Container{ID: "r1", Name: "running-1", State: models.StateRunning}
	stopped := &models.Container{ID: "s1", Name: "stopped-1", State: models.StateStopped}
	db.UpsertContainer(running)
	db.UpsertContainer(stopped)

	runningList, _ := db.ListContainers("running")
	if len(runningList) != 1 {
		t.Errorf("expected 1 running, got %d", len(runningList))
	}

	all, _ := db.ListContainers("")
	if len(all) != 2 {
		t.Errorf("expected 2 total, got %d", len(all))
	}
}

func TestInsertUsage(t *testing.T) {
	db, _ := testDB(t, "usage")

	c := &models.Container{ID: "usage-test", Name: "c", State: models.StateRunning}
	db.UpsertContainer(c)

	if err := db.InsertUsage("usage-test", 0, 1000000, 500000, 10.5, 5.2); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
}

func TestUpdateContainerUsage(t *testing.T) {
	db, _ := testDB(t, "update-usage")

	c := &models.Container{ID: "upd", Name: "upd", State: models.StateRunning, DailyQuotaGB: 100}
	db.UpsertContainer(c)

	if err := db.UpdateContainerUsage("upd", 5000000, 3000000, 25.0, 15.0, 1.5, 0.8); err != nil {
		t.Fatalf("UpdateContainerUsage: %v", err)
	}

	got, err := db.GetContainer("upd")
	if err != nil {
		t.Fatalf("GetContainer: %v", err)
	}
	if got.CurrentRxMbps != 25.0 {
		t.Errorf("expected rx 25.0, got %g", got.CurrentRxMbps)
	}
}

func TestResetDailyUsage(t *testing.T) {
	db, _ := testDB(t, "reset")

	c := &models.Container{ID: "rst", Name: "rst", State: models.StateRunning}
	db.UpsertContainer(c)
	db.UpdateContainerUsage("rst", 1000, 2000, 10, 5, 2.0, 1.0)

	if err := db.ResetDailyUsage(); err != nil {
		t.Fatalf("ResetDailyUsage: %v", err)
	}

	got, _ := db.GetContainer("rst")
	if got.TodayRxGB != 0 || got.TodayTxGB != 0 {
		t.Errorf("expected 0 usage after reset, got rx=%g tx=%g", got.TodayRxGB, got.TodayTxGB)
	}
}

func TestDeleteContainer(t *testing.T) {
	db, _ := testDB(t, "delete")

	c := &models.Container{ID: "del", Name: "del", State: models.StateRunning}
	db.UpsertContainer(c)

	if err := db.DeleteContainer("del"); err != nil {
		t.Fatalf("DeleteContainer: %v", err)
	}

	all, _ := db.ListContainers("")
	if len(all) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(all))
	}
}

func TestConfigGetSet(t *testing.T) {
	db, _ := testDB(t, "kv")

	if err := db.SetConfig("test-key", "test-value"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	val, err := db.GetConfig("test-key")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "test-value" {
		t.Errorf("expected test-value, got %s", val)
	}
}

func TestGetMissingConfig(t *testing.T) {
	db, _ := testDB(t, "missing")

	val, err := db.GetConfig("nonexistent")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty for missing key, got %s", val)
	}
}

func TestUpsertContainerPreservesPortsAndLabels(t *testing.T) {
	db, _ := testDB(t, "json-fields")

	c := &models.Container{
		ID:            "json-test",
		Name:          "json-test",
		State:         models.StateRunning,
		Labels:        map[string]string{"bandwidth.speed": "250"},
		Ports:         []models.PortMapping{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
		Priority:      "premium",
		Webhook:       true,
		History:       false,
		VethInterface: "vethJson0",
	}

	if err := db.UpsertContainer(c); err != nil {
		t.Fatalf("UpsertContainer: %v", err)
	}

	got, err := db.GetContainer("json-test")
	if err != nil {
		t.Fatalf("GetContainer: %v", err)
	}
	if got == nil {
		t.Fatal("expected container, got nil")
	}

	if len(got.Labels) != 1 || got.Labels["bandwidth.speed"] != "250" {
		t.Errorf("labels not preserved: %v", got.Labels)
	}
	if len(got.Ports) != 1 || got.Ports[0].HostPort != 8080 {
		t.Errorf("ports not preserved: %v", got.Ports)
	}
	if got.Priority != "premium" {
		t.Errorf("expected priority premium, got %s", got.Priority)
	}
	if !got.Webhook {
		t.Error("expected webhook=true")
	}
	if got.History {
		t.Error("expected history=false")
	}
}
