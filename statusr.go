// testing gerrit
// maybe this will work
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	//	"os"
	"os/exec"
	//	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	//	"syscall"
	"time"
)

// TBD Support for i3-bar-style signals

func init() {
	// signalchannel := make(chan os.Signal)
	// signal.Notify(signalchannel, syscall.SIGSTOP, syscall.SIGCONT)
	// go func() {
	// 	for {
	// 		select {
	// 		case s := <-signalchannel:
	// 			switch s {
	// 			case syscall.SIGSTOP:
	// 				//				StopComponents()
	// 			case syscall.SIGCONT:
	// 				//				StartComponents()
	// 			}
	// 		}
	// 	}
	// }()
}

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

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func (n *Networker) Get(t time.Time) string {
	type device struct {
		name string
		ips  []string
	}

	out, err := exec.Command("ip", "addr", "show").CombinedOutput()
	if err != nil {
		fmt.Printf("ERR: %q", err)
	}
	devices := make([]device, 0)

	for _, v := range regexp.MustCompile("(?m)^[0-9]+: ").Split(string(out), -1) {
		device := device{}

		if len(v) > 0 {
			lines := strings.Split(v, "\n")
			device.name = strings.Fields(lines[0])[0]
			for k := range lines {
				sections := strings.Fields(lines[k])
				if len(sections) > 0 &&
					strings.HasPrefix(sections[0], "inet") &&
					contains(sections, "global") {
					// check that it's a "global scope", or routabale outside of this laptop.
					device.ips = append(device.ips, sections[1])
				}
			}
			// global connections to not show
			if strings.HasPrefix(device.name, "codebase") ||
				strings.HasPrefix(device.name, "docker0") {
				continue
			}

			devices = append(devices, device)
		}
	}

	addresses := make([]string, 0)

	for _, v := range devices {
		if len(v.ips) > 0 {
			addresses = append(addresses, v.name)
			for _, ip := range v.ips {
				addresses = append(addresses, ip)
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

type Brightnesser struct {
}

func (b *Brightnesser) Get(time.Time) string {
	c, err := ioutil.ReadFile("/sys/class/backlight/intel_backlight/brightness")
	if err != nil {
		return fmt.Sprintf("ERR: %q", err)
	}
	return strings.TrimSpace(string(c))
}

type Mailer struct {
}

func (m *Mailer) Get(time.Time) string {
	out, err := exec.Command("notmuch", "count", "tag:inbox").CombinedOutput()
	if err != nil {
		return fmt.Sprintf("ERR %q", err)
	}

	n := strings.TrimSpace(string(out))
	if n == "" {
		return "m: 0"
	}

	return "m: " + n
}

type CompResult struct {
	Order   int
	Content string
}

func LoopSetTitle(td time.Duration, stdout bool, c <-chan string) {
	var final string
	go func() {
		for _ = range time.Tick(td) {
			if stdout {
				fmt.Printf("%s\n", final)
			} else {
				_, _ = exec.Command("xsetroot", "-name", final).CombinedOutput()
			}

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
	use_stdout := flag.Bool("stdout", false, "Should I print or use xsetroot?")
	flag.Parse()
	comps := make([]*Component, 0)

	// these go in order!
	comps = append(comps, &Component{time.Second, &Networker{}})
	comps = append(comps, &Component{time.Second, &Brightnesser{}})
	comps = append(comps, &Component{time.Second, &Batteryer{}})
	//comps = append(comps, &Component{time.Second, &Volumer{}})
	comps = append(comps, &Component{time.Minute, &Mailer{}})
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

	go LoopSetTitle(delta, *use_stdout, titler)
	CollectAndConstruct(len(comps), c, titler)
}
