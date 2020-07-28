package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	term "github.com/inancgumus/screen"
	"time"
)

var online_cpus = []int{}
var isolated_cpus = []int{}
var isolated_string = []string{}
var monitored_pids = make(map[int]string)
var monitored_string = []string{}

func parseIsolCpus() error {
	var err error

	f, err := os.Open("/tmp/cmdline")
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	line, err := r.ReadString('\n')
	if err != nil {
		fmt.Printf("error reading file %s", err)
		return err
	}
	
	reg := regexp.MustCompile(`\s(isolcpus|rcu_nocbs)=([\d+\-?\d+?\,?]+)`)
	if reg.MatchString(line) {
		match := reg.FindStringSubmatch(line)
		//fmt.Println(match)
		for _, c := range strings.Split(match[2], ",") {
			cs := strings.Split(c, "-")
			//fmt.Printf("%s %d\n", c, len(cs))
			if len(cs) > 1 {
				from, _ := strconv.Atoi(cs[0])
				to, _ := strconv.Atoi(cs[1])
				for i := from; i <= to; i++ {
					isolated_cpus = append(isolated_cpus, i)
				}
			} else {
				cpu, _ := strconv.Atoi(cs[0])
				isolated_cpus = append(isolated_cpus, cpu)
			}

		}
	}

	for _, cpu := range isolated_cpus {
		isolated_string = append(isolated_string, strconv.Itoa(cpu))
	}

	return nil
}

func nProc() error {
	var err error

	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			fmt.Printf("error reading file %s", err)
			break
		}
		
		if strings.HasPrefix(line, "processor") {
			cpu, _ := strconv.Atoi(strings.TrimSpace(strings.Split(line, ":")[1]))
			online_cpus = append(online_cpus, cpu)
		}
	}

	//fmt.Println(online_cpus)
	return nil
}

func isIsolated(cpu int) bool {
	for _, item := range isolated_cpus {
		if item == cpu {
			return true
		}
	}

	return false
}

func isMonitored(pid int) bool {
	_, ok := monitored_pids[pid]

	return ok
}

func findProcCpu(pid int) (int, error) {
	var err error

	path := fmt.Sprintf("/proc/%d/stat", pid)
	//fmt.Println(path)
	proc, err := os.Open(path)
	if err != nil {
		return -1, err
	}
	defer proc.Close()

	p := bufio.NewReader(proc)
	stat, err := p.ReadString('\n')
	if err != nil {
		fmt.Printf("error reading file %s", err)
		return -1, err
	}
	fields := strings.Split(stat, " ")
	cpu, _ := strconv.Atoi(fields[38])

	return cpu, nil
}

func findProcStatus(pid int) (string, error) {
	var err error

	path := fmt.Sprintf("/proc/%d/stat", pid)
	//fmt.Println(path)
	proc, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer proc.Close()

	p := bufio.NewReader(proc)
	stat, err := p.ReadString('\n')
	if err != nil {
		fmt.Printf("error reading file %s", err)
		return "", err
	}
	fields := strings.Split(stat, " ")
	string := fields[2]

	return string, nil
}

func readSchedDebug() error {
	var err error

	f, err := os.Open("/proc/sched_debug")
	if err != nil {
		return err
	}
	defer f.Close()

	reg := regexp.MustCompile(`^>R\s+(\w+)\s+(\d+)`)

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			fmt.Printf("error reading file %s", err)
			break
		}

		if reg.MatchString(line) {
			match := reg.FindStringSubmatch(line)
			pid, _ := strconv.Atoi(match[2])

			cpu, err := findProcCpu(pid)
			if err != nil {
				fmt.Printf("error parsing proc cpu %s", err)
				break
			}

			status, err := findProcStatus(pid)
			if err != nil {
				fmt.Printf("error parsing proc status %s", err)
				break
			}

			if isMonitored(pid) {
				fmt.Printf("comm=%s pid=%d cpu=%d status=%s -- OK\n",
					match[1], pid, cpu, status)
				continue
			}

			if isIsolated(cpu) && !isMonitored(pid) {
				fmt.Printf("comm=%s pid=%d cpu=%d status=%s -- WILL STARVE!\n",
					match[1], pid, cpu, status)
				monitored_pids[pid] = status
				//XXX change pid scheduling class
				fmt.Printf("comm=%s pid=%d cpu=%d status=%s -- CLASS CHANGED\n",
					match[1], pid, cpu, status)
			} else {
				fmt.Printf("comm=%s pid=%d cpu=%d status=%s -- OK\n",
					match[1], pid, cpu, status)
			}

		}
	}

	monitored_string = nil
	for pid, _ := range monitored_pids {
		monitored_string = append(monitored_string, strconv.Itoa(pid))
	}

	return nil
}

func checkMonitored() {
	for pid, _ := range monitored_pids {
		s, err := findProcStatus(pid)
		if err != nil {
			fmt.Printf("Couldn't find proc fs for pid %d -- removing from monitored\n", pid)
			delete(monitored_pids, pid)
		}

		if s != "R" {
			fmt.Printf("Status change for pid %d (%s) -- removing from monitored\n", pid, s)
			delete(monitored_pids, pid)
		}
	}
}

func main() {
	//nProc()
	parseIsolCpus()

	for {
		term.Clear()
		term.MoveTopLeft()

		fmt.Println("----   go monitor (Ctrl+c to exit)  ----")
		fmt.Printf("----   monitoring isolated cpus = %s   ----\n", strings.Join(isolated_string, ","))
		fmt.Printf("----   monitoring pids = %s   ----\n", strings.Join(monitored_string, ","))
		fmt.Println()
		readSchedDebug()
		checkMonitored()

		time.Sleep(time.Second * 3)
	}
}
