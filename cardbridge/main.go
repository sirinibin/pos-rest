// StartERP Card Bridge: connects the shop's card machines (on the shop
// network, USB or a serial cable) to StartERP. It keeps an outgoing
// connection to StartERP, receives "send this amount" jobs from the tills and
// answers with the machine's result. Its own page is http://127.0.0.1:17777.
//
//	cardbridge                 start (opens the page until paired)
//	cardbridge run [--no-browser]
//	cardbridge pair CODE [--server URL] [--name NAME]
//	cardbridge autostart on|off
//	cardbridge status | version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags)
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "StartERP Card Bridge:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "run"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	cfgPath := fs.String("config", defaultConfigPath(), "settings file")
	noBrowser := fs.Bool("no-browser", false, "do not open the page")
	server := fs.String("server", "", "StartERP API address")
	name := fs.String("name", "", "name of this computer")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cs, err := openConfig(*cfgPath)
	if err != nil {
		return err
	}
	if cmd == "run" {
		logToFile(*cfgPath)
	}
	switch cmd {
	case "version":
		fmt.Println(Version)
		return nil
	case "status":
		c := cs.get()
		fmt.Printf("version %s\nsettings %s\npaired %v\nstore %s\nserver %s\nstart with the computer %v\ndrivers %v\n",
			Version, *cfgPath, c.paired(), c.StoreName, c.Server, autostartInstalled(), driverIDs())
		return nil
	case "autostart":
		if fs.Arg(0) == "off" {
			return uninstallAutostart()
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return installAutostart(exe)
	case "pair":
		code := fs.Arg(0)
		if code == "" {
			return errors.New("usage: cardbridge pair CODE [--server URL] [--name NAME]")
		}
		srv := cs.get().Server
		if *server != "" {
			s, ok := validServer(*server)
			if !ok {
				return errors.New("the server address must start with https://")
			}
			srv = s
		}
		nm := cs.get().Name
		if *name != "" {
			nm = *name
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		a, err := newClient(srv, "").pair(ctx, code, nm)
		if err != nil {
			return err
		}
		if err := cs.update(func(c *Config) {
			c.Server, c.Token, c.BridgeID, c.StoreID, c.StoreName, c.Name = srv, a.Token, a.BridgeID, a.StoreID, a.StoreName, nm
		}); err != nil {
			return err
		}
		fmt.Println("Paired with", a.StoreName)
		return nil
	case "run":
		return serve(cs, !*noBrowser)
	}
	return fmt.Errorf("unknown command %q", cmd)
}

func serve(cs *configStore, browser bool) error {
	l := &ringLog{}
	w := newWorker(cs, l)
	u := newUI(cs, w, l)
	pageURL := "http://127.0.0.1:" + strconv.Itoa(u.port) + "/"
	ln, err := u.listen()
	if err != nil {
		// already running: show its page
		if isAddrInUse(err) {
			if browser {
				openBrowser(pageURL)
			}
			fmt.Println("The Card Bridge is already running:", pageURL)
			return nil
		}
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Handler: u.handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	l.add("Card Bridge " + Version + " started on " + osName() + "; page " + pageURL)
	if browser && !cs.get().paired() {
		openBrowser(pageURL)
	}
	w.loop(ctx)
	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Shutdown(sctx)
}

// logToFile writes the log next to the settings (the Windows build has no
// console); it starts over once it passes 2 MB.
func logToFile(cfgPath string) {
	p := filepath.Join(filepath.Dir(cfgPath), "cardbridge.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 2<<20 {
		_ = os.Rename(p, p+".old")
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
	}
}

func isAddrInUse(err error) bool {
	var oe *net.OpError
	if errors.As(err, &oe) {
		var se *os.SyscallError
		if errors.As(oe.Err, &se) {
			return errors.Is(se.Err, syscall.EADDRINUSE) || se.Err.Error() == "bind: Only one usage of each socket address (protocol/network address/port) is normally permitted."
		}
	}
	return false
}
