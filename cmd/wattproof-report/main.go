// Command wattproof-report is the rightsizing report (ROADMAP Phase 1R step 4),
// first cut: it reads requests and usage per workload from a trace file, not
// yet from Prometheus. It recommends a CPU request per hour of the week and a
// memory request (ADR-0013), reports what they would free at the peak, and
// estimates energy with internal/sim on the cluster's nodes. It never acts.
//
//	go run ./cmd/wattproof-report -workloads w.csv -nodes n.csv [-curves testdata] [-out rec.csv]
//
// The workload file has one row per workload and hour in which it ran:
//
//	workload,hour,cpu_use,cpu_peak,cpu_req,mem_use,mem_req,exclude
//
// hour counts from 0 at the start of a week; cpu_use and mem_use are the
// hour's means, cpu_peak its highest 5 minutes; exclude is empty or the reason
// the workload must not be rightsized (Guaranteed QoS, VPA, Utilization HPA).
// The node file has one row per worker:
//
//	node,cpus,mem,curve,local_storage_pod
//
// curve names a power curve file ("load,watts", as in testdata) in -curves;
// local_storage_pod names a pod on node-local storage, which keeps the node on
// (invariant 1). CPU and memory use the same units in both files.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	workloads := flag.String("workloads", "", "CSV of requests and usage per workload and hour")
	nodes := flag.String("nodes", "", "CSV of the cluster's workers")
	curves := flag.String("curves", "testdata", "directory with the power curves the node file names")
	boot := flag.Duration("boot", 5*time.Minute, "time from power-on until a node is Ready")
	out := flag.String("out", "", "CSV to write the recommendations to (optional)")
	flag.Parse()
	if *workloads == "" || *nodes == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*workloads, *nodes, *curves, *boot, *out); err != nil {
		fmt.Fprintln(os.Stderr, "wattproof-report:", err)
		os.Exit(1)
	}
}

func run(workloadFile, nodeFile, curveDir string, boot time.Duration, outFile string) error {
	wf, err := os.Open(workloadFile)
	if err != nil {
		return err
	}
	defer wf.Close()
	ws, hours, err := readWorkloads(wf)
	if err != nil {
		return fmt.Errorf("%s: %w", workloadFile, err)
	}
	nf, err := os.Open(nodeFile)
	if err != nil {
		return err
	}
	defer nf.Close()
	ns, err := readNodes(nf)
	if err != nil {
		return fmt.Errorf("%s: %w", nodeFile, err)
	}
	rep, err := build(ws, hours, ns, curveDir, boot)
	if err != nil {
		return err
	}
	rep.write(os.Stdout)
	if outFile == "" {
		return nil
	}
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	if err := rep.writeRecommendations(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
