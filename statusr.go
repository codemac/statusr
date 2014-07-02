// testing gerrit
// maybe this will work
package main

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Component struct {
	Delta time.Duration
	Comp  StringGetter
}

type StringGetter interface {
	Get(time.Time) string
}

type Timer struct {
}

func (timer *Timer) Get(t time.Time) string {
	return t.Format("Monday 2006-01-02 15:04:05")
}

type Networker struct {
}

func (n *Networker) Get(t time.Time) string {
	out, err := exec.Command("nmcli", "-t", "-f", "device", "dev").CombinedOutput()
	if err != nil {
		fmt.Printf("ERR: %q", err)
	}

	devices := make([]string, 0)
	for _, v := range strings.Split(string(out), "\n") {
		if v != "" {
			devices = append(devices, v)
		}
	}

	addresses := make([]string, 0)
	for _, d := range devices {
		out, err = exec.Command("nmcli", "-t", "-f", "IP4", "dev",
			"list", "iface", d).CombinedOutput()
		if err != nil {
			fmt.Printf("ERR: %q", err)
		}

		for _, v := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(v, "IP4.ADDRESS") {
				address := d + ": " + strings.TrimRight(strings.Fields(v)[2], ",")
				addresses = append(addresses, address)
			}
		}
	}

	return strings.Join(addresses, " ")
}

type Batteryer struct {
}

func (b *Batteryer) Get(time.Time) string {
	// /sys/class/power_supply/*/uevent

	batteries, err := filepath.Glob("/sys/class/power_supply/BAT*/uevent")
	if err != nil {
		return fmt.Sprintf("ERR: %q", err)
	}

	capacities := make([]string, 0)
	for _, v := range batteries {
		out, err := ioutil.ReadFile(v)
		if err != nil {
			return fmt.Sprintf("ERR: %q", err)
		}

		for _, v := range strings.Split(string(out), "\n") {
			f := strings.Split(v, "=")
			if f[0] == "POWER_SUPPLY_CAPACITY" {
				capacities = append(capacities, f[1])
			}
		}
	}

	ac, err := filepath.Glob("/sys/class/power_supply/AC*/uevent")
	if err != nil {
		return fmt.Sprintf("ERR: %q", err)
	}

	plugs := make([]string, 0)
	for _, v := range ac {
		out, err := ioutil.ReadFile(v)
		if err != nil {
			return fmt.Sprintf("ERR: %q", err)
		}

		for _, v := range strings.Split(string(out), "\n") {
			f := strings.Split(v, "=")
			if f[0] == "POWER_SUPPLY_ONLINE" {
				plugs = append(plugs, f[1])
			}
		}
	}

	plugs = append(plugs, capacities...)
	return strings.Join(plugs, " ")
}

type Volumer struct {
}

func (v *Volumer) Get(time.Time) string {
	out, err := exec.Command("pactl", "list", "sinks").CombinedOutput()
	if err != nil {
		return fmt.Sprintf("ERR %q", err)
	}

	muted := ""
	vol := ""
	for _, v := range strings.Split(string(out), "\n") {
		f := strings.Fields(v)
		if len(f) > 0 {
			switch f[0] {
			case "Volume:":
				vol = f[4]
			case "Mute:":
				if f[1] == "yes" {
					muted = "[M]"
				}
			}
		}
	}

	return muted + " " + vol

}

type CompResult struct {
	Order   int
	Content string
}

func LoopSetTitle(td time.Duration, c <-chan string) {
	var final string
	go func() {
		for _ = range time.Tick(td) {
			_, _ = exec.Command("xsetroot", "-name", final).CombinedOutput()
			//fmt.Printf("%s\n", final)
		}
	}()
	for {
		select {
		case s := <-c:
			final = s
		}
	}
}

func Construct(st []string) string {
	var final string

	for k, s := range st {
		final = final + s
		if k != len(st)-1 {
			final = final + " | "
		}
	}

	return final
}

func CollectAndConstruct(total int, c <-chan CompResult, titler chan<- string) {
	statuses := make([]string, total)
	for {
		select {
		case cr := <-c:
			if cr.Order < total {
				statuses[cr.Order] = cr.Content
			}
		}

		titler <- Construct(statuses)
	}

}

func RunComponent(order int, cmp *Component, cr chan<- CompResult) {
	for clock := range time.Tick(cmp.Delta) {
		cr <- CompResult{
			Content: cmp.Comp.Get(clock),
			Order:   order,
		}
	}
}

func main() {

	comps := make([]*Component, 0)

	// these go in order!
	comps = append(comps, &Component{time.Second, &Networker{}})
	comps = append(comps, &Component{time.Second, &Batteryer{}})
	//comps = append(comps, &Component{time.Second, &Volumer{}})
	comps = append(comps, &Component{time.Second, &Timer{}})

	c := make(chan CompResult)
	titler := make(chan string)
	for k, cmp := range comps {
		go RunComponent(k, cmp, c)
	}

	delta := time.Hour
	for _, v := range comps {
		if v.Delta < delta {
			delta = v.Delta
		}
	}

	go LoopSetTitle(delta, titler)
	CollectAndConstruct(len(comps), c, titler)
}
