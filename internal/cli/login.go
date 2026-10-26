package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/havspect/tstamps/internal/client"
)

func (a *App) login(args []string) int {
	fs := newFlags("login")
	urlFlag, _ := clientFlags(fs)
	email := fs.String("email", "", "email")
	password := fs.String("password", "", "password")
	noBrowser := fs.Bool("no-browser", false, "do not open browser automatically")
	if err := parseFlags(fs, args); err != nil {
		return a.flagError("login", err)
	}
	if code, ok := a.checkNoArgs("login", fs); !ok {
		return code
	}

	cfg, err := a.LoadConfig(*urlFlag, "")
	if err != nil {
		return a.fail("login", err)
	}
	if cfg.URL == "" {
		return a.usageLine("tstamps login --url URL [--email x] [--password y] [--no-browser]")
	}

	c := a.NewClient(client.Config{URL: cfg.URL})

	if *email != "" || *password != "" {
		in := bufio.NewReader(a.In)
		if *email == "" {
			fmt.Fprint(a.Out, "email: ")
			if *email, err = readLine(in); err != nil {
				return a.fail("login", err)
			}
		}
		if *password == "" {
			fmt.Fprint(a.Out, "password: ")
			if *password, err = a.readPassword(in); err != nil {
				return a.fail("login", err)
			}
		}
		me, err := c.Login(*email, *password)
		if err != nil {
			return a.fail("login", err)
		}
		tok, err := c.CreateToken("tstamps login")
		if err != nil {
			return a.fail("login", err)
		}
		cfg.Token = tok.Token
		cfg.RefreshToken = ""
		if err := a.SaveConfig(cfg); err != nil {
			return a.fail("login", err)
		}
		fmt.Fprintf(a.Out, "logged in as %s <%s>\n", me.Name, me.Email)
		return 0
	}

	// OAuth 2.1 Device Grant Flow
	deviceAuth, err := c.StartDeviceAuth()
	if err != nil {
		return a.fail("login", err)
	}

	fmt.Fprintf(a.Out, "Open %s and confirm code %s\n", deviceAuth.VerificationURI, deviceAuth.UserCode)

	if !*noBrowser {
		uri := deviceAuth.VerificationURIComplete
		if uri == "" {
			uri = deviceAuth.VerificationURI
		}
		go openBrowser(uri)
	}

	tokenSet, err := c.PollDeviceToken(context.Background(), deviceAuth.DeviceCode, time.Duration(deviceAuth.Interval)*time.Second)
	if err != nil {
		return a.fail("login", err)
	}

	cfg.Token = tokenSet.AccessToken
	cfg.RefreshToken = tokenSet.RefreshToken
	if err := a.SaveConfig(cfg); err != nil {
		return a.fail("login", err)
	}

	// Re-create client with new token to fetch me
	c2 := a.NewClient(cfg)
	me, err := c2.Me()
	if err != nil {
		return a.fail("login", err)
	}

	fmt.Fprintf(a.Out, "logged in as %s <%s>\n", me.Name, me.Email)
	return 0
}

func openBrowser(urlStr string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", urlStr)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", urlStr)
	default:
		cmd = exec.Command("xdg-open", urlStr)
	}
	_ = cmd.Start()
}

// readPassword turns echo off when stdin is a terminal, so the password stays out
// of the scrollback; piped input is not echoed anyway.
func (a *App) readPassword(in *bufio.Reader) (string, error) {
	f, ok := a.In.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return readLine(in)
	}
	b, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(a.Out) // ReadPassword does not echo the Enter.
	return string(b), err
}

// readLine returns one line without its terminator; EOF with bytes still counts.
func readLine(in *bufio.Reader) (string, error) {
	s, err := in.ReadString('\n')
	if err != nil && s == "" {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}
