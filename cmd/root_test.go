package cmd

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

func TestInitialDelay(t *testing.T) {
	for _, tt := range []struct{ uptime, want time.Duration }{
		{0, time.Hour}, {30 * time.Minute, 30 * time.Minute}, {time.Hour, 0}, {2 * time.Hour, 0},
	} {
		if got := initialDelay(time.Hour, tt.uptime); got != tt.want {
			t.Fatalf("uptime %s: got %s, want %s", tt.uptime, got, tt.want)
		}
	}
}

func TestTouchSentinel(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "reboot-required")
	if err := touchSentinel(sentinel); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(sentinel)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("not a regular sentinel: %v", err)
	}
	if err := os.WriteFile(sentinel, []byte("keep existing contents"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := touchSentinel(sentinel); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(sentinel)
	if err != nil || string(contents) != "keep existing contents" {
		t.Fatalf("sentinel modified: %q, %v", contents, err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(sentinel, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, fifo, dir, filepath.Join(dir, "missing", "sentinel")} {
		if err := touchSentinel(path); err == nil {
			t.Fatalf("expected error for %s", path)
		}
	}
}

func TestRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sentinel := filepath.Join(t.TempDir(), "reboot-required")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := run(ctx, time.Hour, sentinel, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
			t.Fatalf("canceled run created sentinel: %v", err)
		}

		ctx, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		if err := run(ctx, time.Hour, sentinel, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
			t.Fatalf("marker created before threshold: %v", err)
		}

		ctx, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		if err := run(ctx, 10*time.Millisecond, sentinel, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(sentinel); err != nil {
			t.Fatal(err)
		}

		ctx, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		dueSentinel := filepath.Join(t.TempDir(), "due")
		if err := run(ctx, time.Hour, dueSentinel, 2*time.Hour); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dueSentinel); err != nil {
			t.Fatalf("overdue node not marked: %v", err)
		}

		if err := run(context.Background(), time.Hour, filepath.Join(t.TempDir(), "missing", "sentinel"), time.Hour); err == nil {
			t.Fatal("expected sentinel error")
		}
	})
}

func TestRunRecreatesRemovedSentinel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sentinel := filepath.Join(t.TempDir(), "reboot-required")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- run(ctx, time.Second, sentinel, time.Second) }()
		synctest.Wait()
		if err := os.Remove(sentinel); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if _, err := os.Stat(sentinel); err != nil {
			t.Fatal(err)
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

func TestRootRejectsInvalidFlags(t *testing.T) {
	oldPeriod, oldSentinel := period, rebootSentinel
	defer func() { period, rebootSentinel = oldPeriod, oldSentinel }()
	for _, value := range []time.Duration{0, -time.Second} {
		period, rebootSentinel = value, filepath.Join(t.TempDir(), "reboot-required")
		if err := root(rootCmd, nil); err == nil {
			t.Fatalf("accepted period %s", value)
		}
	}
	period, rebootSentinel = time.Hour, "relative-path"
	if err := root(rootCmd, nil); err == nil {
		t.Fatal("accepted relative sentinel")
	}
}
