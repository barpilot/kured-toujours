// Copyright © 2018 Guilhem Lettron
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "kured-toujours",
	Short:         "Mark a node for reboot after a configured uptime",
	Long:          "Create a reboot sentinel after the uptime threshold. A separate reboot controller such as kured must consume it.",
	Args:          cobra.NoArgs,
	RunE:          root,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	period         time.Duration
	rebootSentinel string
)

func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().DurationVar(&period, "period", time.Hour, "minimum node uptime before requesting a reboot")
	rootCmd.Flags().StringVar(&rebootSentinel, "reboot-sentinel", "/var/run/reboot-required", "absolute path to the reboot sentinel in this filesystem")
}

func root(cmd *cobra.Command, _ []string) error {
	if period <= 0 {
		return fmt.Errorf("period must be positive")
	}
	if !filepath.IsAbs(rebootSentinel) {
		return fmt.Errorf("reboot-sentinel must be an absolute path")
	}
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return fmt.Errorf("read uptime: %w", err)
	}
	return run(cmd.Context(), period, rebootSentinel, time.Duration(info.Uptime)*time.Second)
}

func initialDelay(period, uptime time.Duration) time.Duration {
	if uptime >= period {
		return 0
	}
	return period - uptime
}

func run(ctx context.Context, period time.Duration, sentinel string, uptime time.Duration) error {
	delay := initialDelay(period, uptime)
	log.Printf("Request reboot in %s using %s", delay, sentinel)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			if ctx.Err() != nil {
				return nil
			}
			if err := touchSentinel(sentinel); err != nil {
				return fmt.Errorf("create sentinel %s: %w", sentinel, err)
			}
			log.Printf("Reboot sentinel present: %s", sentinel)
			timer.Reset(period)
		}
	}
}

func touchSentinel(path string) error {
	// Do not follow a final symlink, truncate existing contents, or block on a FIFO.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return fmt.Errorf("sentinel must be a regular file")
	}
	return f.Close()
}
