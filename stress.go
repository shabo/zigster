package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// stressTargets lists what we can stress and how.
var stressTargets = []struct {
	name string
	desc string
}{
	{"cpu", "All CPU cores (stress-ng --cpu)"},
	{"gpu", "NVIDIA GPU compute (nvidia-smi)"},
	{"nvme", "NVMe SSD random read/write (fio)"},
	{"disk", "SATA HDD sequential I/O (fio)"},
	{"wifi", "Network interface flood (ping flood)"},
	{"all", "Everything at once"},
}

func runStress(args []string) {
	if len(args) == 0 {
		printStressHelp()
		return
	}

	target := strings.ToLower(args[0])

	duration := 60 * time.Second
	if len(args) > 1 {
		if d, err := time.ParseDuration(args[1]); err == nil {
			duration = d
		} else if secs, err := strconv.Atoi(args[1]); err == nil {
			duration = time.Duration(secs) * time.Second
		}
	}

	durSecs := int(duration.Seconds())
	if durSecs < 1 {
		durSecs = 60
	}

	fmt.Printf("Stressing: %s for %ds\n", target, durSecs)
	fmt.Println("Press Ctrl+C to stop early")
	fmt.Println()

	// Handle Ctrl+C to kill child processes
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	switch target {
	case "cpu":
		stressCPU(durSecs, sigCh)
	case "gpu":
		stressGPU(durSecs, sigCh)
	case "nvme":
		stressNVMe(durSecs, sigCh)
	case "disk":
		stressDisk(durSecs, sigCh)
	case "wifi", "net", "network":
		stressWifi(durSecs, sigCh)
	case "all":
		stressAll(durSecs, sigCh)
	default:
		fmt.Fprintf(os.Stderr, "Unknown target: %s\n\n", target)
		printStressHelp()
		os.Exit(1)
	}
}

func printStressHelp() {
	fmt.Println("Usage: sensors stress <target> [duration]")
	fmt.Println()
	fmt.Println("Targets:")
	for _, t := range stressTargets {
		fmt.Printf("  %-8s  %s\n", t.name, t.desc)
	}
	fmt.Println()
	fmt.Println("Duration: e.g. '60' (seconds), '2m', '30s' (default: 60s)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  sensors stress cpu 30s")
	fmt.Println("  sensors stress gpu 2m")
	fmt.Println("  sensors stress all 60")
}

// ── CPU stress ───────────────────────────────────────────────────────

func stressCPU(secs int, sigCh chan os.Signal) {
	if !checkTool("stress-ng") {
		// Fallback: pure Go CPU burn
		fmt.Println("stress-ng not found, using built-in CPU burner")
		cpuBurnFallback(secs, sigCh)
		return
	}

	cpus := runtime.NumCPU()
	fmt.Printf("  stress-ng --cpu %d --timeout %ds\n", cpus, secs)
	runCmd(sigCh, "stress-ng", "--cpu", strconv.Itoa(cpus), "--timeout", fmt.Sprintf("%ds", secs))
}

func cpuBurnFallback(secs int, sigCh chan os.Signal) {
	cpus := runtime.NumCPU()
	fmt.Printf("  burning %d cores for %ds\n", cpus, secs)

	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
		case <-time.After(time.Duration(secs) * time.Second):
		}
		close(done)
	}()

	for i := 0; i < cpus; i++ {
		go func() {
			x := 0.0
			for {
				select {
				case <-done:
					return
				default:
					x += 1.1
					x *= 0.9
				}
			}
		}()
	}

	<-done
	fmt.Println("  done")
}

// ── GPU stress ───────────────────────────────────────────────────────

func stressGPU(secs int, sigCh chan os.Signal) {
	// Try tools in order of how hard they actually push the GPU
	fmt.Printf("  GPU stress for %ds\n", secs)

	// 1. glmark2 — real OpenGL rendering benchmark, pushes GPU hard
	if checkTool("glmark2") {
		fmt.Println("  glmark2 (OpenGL rendering benchmark)")
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			fmt.Println("  note: needs a graphical session (DISPLAY or WAYLAND_DISPLAY)")
		}
		runCmdWithTimeout(secs, sigCh, "glmark2", "--run-forever")
		return
	}

	// 2. glxgears — lighter but still GPU rendering
	if checkTool("glxgears") {
		fmt.Println("  glxgears (OpenGL rendering)")
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			fmt.Println("  note: needs a graphical session (DISPLAY or WAYLAND_DISPLAY)")
		}
		runCmdWithTimeout(secs, sigCh, "glxgears")
		return
	}

	// 3. Fallback: parallel nvidia-smi queries (very light, but something)
	if checkTool("nvidia-smi") {
		fmt.Println("  nvidia-smi query loop (light GPU load)")
		fmt.Println("  tip: install glmark2 for real GPU stress: sudo pacman -S glmark2")
		done := make(chan struct{})
		go func() {
			select {
			case <-sigCh:
			case <-time.After(time.Duration(secs) * time.Second):
			}
			close(done)
		}()

		for {
			select {
			case <-done:
				fmt.Println("  done")
				return
			default:
				exec.Command("nvidia-smi", "--query-gpu=temperature.gpu", "--format=csv,noheader").Output()
			}
		}
	}

	fmt.Fprintln(os.Stderr, "  no GPU stress tool found")
	fmt.Fprintln(os.Stderr, "  install: sudo pacman -S glmark2")
}

// ── NVMe stress ──────────────────────────────────────────────────────

func stressNVMe(secs int, sigCh chan os.Signal) {
	if !checkTool("fio") {
		fmt.Fprintln(os.Stderr, "  fio not found — install: sudo pacman -S fio")
		return
	}

	// Find NVMe device
	dev := findBlockDev("nvme")
	if dev == "" {
		fmt.Fprintln(os.Stderr, "  no NVMe device found")
		return
	}

	fmt.Printf("  fio random read/write on %s for %ds\n", dev, secs)
	fmt.Println("  (using temp file, safe — no raw device writes)")

	tmpDir := "/tmp/sensors-stress-nvme"
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	runCmd(sigCh, "fio",
		"--name=nvme-stress",
		"--directory="+tmpDir,
		"--rw=randrw",
		"--bs=4k",
		"--size=1G",
		"--numjobs=4",
		"--iodepth=32",
		"--ioengine=libaio",
		"--direct=1",
		"--runtime="+strconv.Itoa(secs),
		"--time_based",
		"--group_reporting",
	)
}

// ── Disk (SATA) stress ───────────────────────────────────────────────

func stressDisk(secs int, sigCh chan os.Signal) {
	if !checkTool("fio") {
		fmt.Fprintln(os.Stderr, "  fio not found — install: sudo pacman -S fio")
		return
	}

	fmt.Printf("  fio sequential I/O on /tmp for %ds\n", secs)

	tmpDir := "/tmp/sensors-stress-disk"
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	runCmd(sigCh, "fio",
		"--name=disk-stress",
		"--directory="+tmpDir,
		"--rw=readwrite",
		"--bs=128k",
		"--size=512M",
		"--numjobs=2",
		"--iodepth=8",
		"--ioengine=libaio",
		"--direct=1",
		"--runtime="+strconv.Itoa(secs),
		"--time_based",
		"--group_reporting",
	)
}

// ── WiFi / network stress ────────────────────────────────────────────

func stressWifi(secs int, sigCh chan os.Signal) {
	// Generate network traffic to heat up the WiFi adapter
	// Use parallel downloads from a reliable source, or ping flood

	fmt.Printf("  Network stress for %ds\n", secs)

	// Try iperf3 to localhost first (self-loop, saturates the NIC)
	if checkTool("iperf3") {
		fmt.Println("  iperf3 self-test (loopback stress)")
		// Start server in background
		server := exec.Command("iperf3", "-s", "-D", "-1")
		server.Start()
		time.Sleep(500 * time.Millisecond)

		runCmd(sigCh, "iperf3", "-c", "127.0.0.1", "-t", strconv.Itoa(secs), "-P", "8")
		return
	}

	// Fallback: parallel curl / wget downloads or sustained ping
	fmt.Println("  Sustained ping flood (requires network)")
	runCmd(sigCh, "ping", "-f", "-c", strconv.Itoa(secs*1000), "-i", "0.001", "1.1.1.1")
}

// ── All at once ──────────────────────────────────────────────────────

func stressAll(secs int, sigCh chan os.Signal) {
	fmt.Printf("  Stressing ALL components for %ds\n\n", secs)

	type job struct {
		name string
		fn   func(int, chan os.Signal)
	}

	jobs := []job{
		{"CPU", stressCPU},
		{"GPU", stressGPU},
		{"NVMe", stressNVMe},
		{"WiFi", stressWifi},
	}

	// Create per-job signal channels
	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
		case <-time.After(time.Duration(secs) * time.Second):
		}
		close(done)
	}()

	for _, j := range jobs {
		j := j
		jobSig := make(chan os.Signal, 1)
		go func() {
			<-done
			jobSig <- syscall.SIGTERM
		}()
		go func() {
			fmt.Printf("── Starting %s stress ──\n", j.name)
			j.fn(secs, jobSig)
		}()
	}

	<-done
	time.Sleep(500 * time.Millisecond) // let children clean up
	fmt.Println("\n  All stress tests complete")
}

// ── Helpers ──────────────────────────────────────────────────────────

func checkTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func findBlockDev(prefix string) string {
	out, err := exec.Command("lsblk", "-d", "-n", "-o", "NAME,TYPE").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "disk" && strings.HasPrefix(fields[0], prefix) {
			return "/dev/" + fields[0]
		}
	}
	return ""
}

// runCmdWithTimeout runs a command that doesn't have its own timeout mechanism.
// It kills the process after the given duration.
func runCmdWithTimeout(secs int, sigCh chan os.Signal, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "  failed to start %s: %v\n", name, err)
		return
	}

	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	select {
	case err := <-cmdDone:
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", name, err)
		} else {
			fmt.Println("  completed")
		}
	case <-sigCh:
		cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(200 * time.Millisecond)
		cmd.Process.Kill()
		fmt.Println("\n  interrupted")
	case <-time.After(time.Duration(secs) * time.Second):
		cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(200 * time.Millisecond)
		cmd.Process.Kill()
		fmt.Println("  completed")
	}
}

func runCmd(sigCh chan os.Signal, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "  failed to start %s: %v\n", name, err)
		return
	}

	// Wait for either the command to finish or a signal
	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	select {
	case err := <-cmdDone:
		if err != nil {
			// Exit code 1 from stress tools is normal (timeout)
			if exitErr, ok := err.(*exec.ExitError); ok {
				if exitErr.ExitCode() == 1 {
					fmt.Println("  completed")
					return
				}
			}
			fmt.Fprintf(os.Stderr, "  %s: %v\n", name, err)
		} else {
			fmt.Println("  completed")
		}
	case <-sigCh:
		cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(200 * time.Millisecond)
		cmd.Process.Kill()
		fmt.Println("\n  interrupted")
	}
}
