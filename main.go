// sermon-proof: a local tool for proofreading sermon transcripts against audio.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"sermonproof/internal/server"
)

func main() {
	port := flag.Int("port", 0, "port on 127.0.0.1 (0 = pick a free one)")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: sermon-proof [-port N] [-no-open] [檔名.md [音檔.mp3]]")
		flag.PrintDefaults()
	}
	flag.Parse()

	sess := &server.Session{}
	var md, audio string
	for _, a := range flag.Args() {
		if strings.HasSuffix(strings.ToLower(a), ".md") {
			md = a
		} else {
			audio = a
		}
	}
	if md != "" {
		if err := sess.OpenMD(md); err != nil {
			fmt.Fprintln(os.Stderr, "無法開啟:", err)
			os.Exit(1)
		}
	}
	if audio != "" {
		if err := sess.SetAudio(audio); err != nil {
			fmt.Fprintln(os.Stderr, "無法使用音檔:", err)
			os.Exit(1)
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(*port))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	srv := server.New(sess)
	srv.SetPort(p)

	url := "http://127.0.0.1:" + p + "/"
	fmt.Println("sermon-proof 已啟動:", url, "(Ctrl+C 結束)")
	if !*noOpen {
		_ = exec.Command("open", url).Start()
	}
	if err := http.Serve(ln, srv.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
