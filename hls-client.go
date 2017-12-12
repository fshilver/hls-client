package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kz26/m3u8"
)

type configInfo struct {
	fileName    string
	destIP      string
	serviceCode string
	contentType string
	bitrateType string
}


func glbSetup(u *url.URL, c *http.Client) (*url.URL, error) {

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	//req.Header.Set("User-Agent", "dahakan")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 301 {
		return nil, fmt.Errorf("Received HTTP %v for %v", resp.StatusCode, u.String())
	}

	uri, err := u.Parse(resp.Header.Get("Location"))
	if err != nil {
		return nil, err
	}

	return uri, err

}

func getContent(u *url.URL, c *http.Client) (io.ReadCloser, *url.URL, error) {

	start := time.Now()

	//	log.Println(u.String())
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	//req.Header.Set("User-Agent", "dahakan")
	resp, err := c.Do(req)
	if err != nil {
		//log.Println("error:", u.String(), "delay:", (int(time.Now().Sub(start)) / 1000000))
		return nil, nil, err
	}

	delay := (int(time.Now().Sub(start)) / 1000000)
	if delay >= 1500 {
		log.Println(u.String(), "delay:", delay)
	}

	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("Received HTTP %v for %v", resp.StatusCode, u.String())
	}

	resurl := resp.Request

	return resp.Body, resurl.URL, err

}

func absolutize(rawurl string, u *url.URL) (uri *url.URL, err error) {
	suburl := rawurl
	uri, err = u.Parse(suburl)
	if err != nil {
		return
	}

	if rawurl == u.String() {
		return
	}

	if !uri.IsAbs() { // relative URI
		if rawurl[0] == '/' { // from the root
			suburl = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, rawurl)
		} else { // last element
			splitted := strings.Split(u.String(), "/")
			splitted[len(splitted)-1] = rawurl

			suburl = strings.Join(splitted, "/")
		}
	}

	suburl, err = url.QueryUnescape(suburl)
	if err != nil {
		return
	}

	uri, err = u.Parse(suburl)
	if err != nil {
		return
	}

	return
}

func download(u *url.URL, c *http.Client, f float64, d int) {

	start := time.Now()

	content, _, err := getContent(u, c)

	t := int(time.Now().Sub(start)) / 1000000

	if t > 5000 {
		log.Println(t, " ", u.String())
	}

	start = time.Now()

	if err != nil {
		log.Println("error:", err, "url:", u.String())
		return
	}

	for {
		buf := make([]byte, 32*1024)
		_, err := content.Read(buf)

		if err != nil && err != io.EOF {
			log.Println("error:", err, "url:", u.String())
			break
		}

		if err == io.EOF {
			break
		}
	}
	content.Close()

	restime := int(f*1000) - (int(time.Now().Sub(start)) / 1000000)

	if d > 0 {
		//restime = d
		restime = d - (int(time.Now().Sub(start)) / 1000000)
		//log.Println(" : ", restime)
	}

	t = int(time.Now().Sub(start)) / 1000000

	if t > 5000 {
		log.Println(t, " ", u.String())
	}

	if restime > 0 {
		time.Sleep(time.Duration(restime * 1000000))

	}

}

func getPlaylist(u *url.URL, t int, c *http.Client, d int) {

	for t > 0 {

		content, _, err := getContent(u, c)
		if err != nil {
			log.Println("error3:", err)
			break
		}

		playlist, listType, err := m3u8.DecodeFrom(content, true)
		if err != nil {
			log.Println("error:", err)
			break
		}
		content.Close()

		if listType != m3u8.MEDIA && listType != m3u8.MASTER {
			log.Println("error: Not a valid playlist")
			break
		}

		if listType == m3u8.MEDIA {

			mediapl := playlist.(*m3u8.MediaPlaylist)

			for idx, segment := range mediapl.Segments {
				if segment == nil {
					chunk := mediapl.Segments[idx-1]
					if chunk != nil {
						msURL, err := absolutize(chunk.URI, u)
						if err != nil {
							log.Println("error:", err)
							break
						}
						download(msURL, c, chunk.Duration, d)
						if false {
							t -= d
							log.Println("time : ", t)
						} else {
							t -= int(chunk.Duration)
							//	log.Println("time2 : ", t)
						}
						break
					}
				}
			}
		} else {
			log.Println("error: invaild m3u8 Type")
			break
		}
	}
}

type DialerFunc func(net, addr string) (net.Conn, error)

func make_dialer(keepAlive bool, clientIP string) DialerFunc {
	return func(network, addr string) (net.Conn, error) {

		localAddr, err := net.ResolveIPAddr("ip", clientIP)
		if err != nil {
			return nil, err
		}

		LocalBindAddr := &net.TCPAddr{IP: localAddr.IP}

		conn, err := (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			LocalAddr: LocalBindAddr,
		}).Dial(network, addr)
		if err != nil {
			return conn, err
		}
		if !keepAlive {
			conn.(*net.TCPConn).SetLinger(0)
		}
		return conn, err
	}
}

func main() {

	FileName := flag.String("filename", "", "generation info file name. ")
	Address := flag.String("addr", "", "gslb server addresss. (ex) 127.0.0.1:18085")
	SessionCount := flag.Int("count", 0, "the number of session. default is generation info file count")
	Interval := flag.Int("interval", 1, "session generation interval (second)")
	PlayTime := flag.Int("playtime", 900, "play time (second)")
	Delay := flag.Int("delay", 0, "request chunk delay(millisec)")

	flag.Parse()

	if *FileName == "" || *Address == "" {
		log.Println("HLSGenerator v1.0.2")
		flag.Usage()
		return
	}

	configData, err := ioutil.ReadFile(*FileName)
	if err != nil {
		log.Println("config file read file: ", err)
		return
	}

	cfData := string(configData)

	token := strings.Split(cfData, "\n")

	var cfglist []configInfo

	i := 0
	for i < len(token) {
		if token[i] != "" {
			data := strings.Fields(token[i])
			if len(data) != 5 {
				log.Println("invalid config data : ", token[i])
				i++
				continue
			}
			cfg := configInfo{}

			cfg.fileName = data[0]
			cfg.destIP = data[1]
			cfg.serviceCode = data[2]
			cfg.contentType = data[3]
			cfg.bitrateType = data[4]

			cfglist = append(cfglist, cfg)
		}
		i++
	}

	if len(cfglist) == 0 {
		log.Println("cfglist is zero.")
		return
	}

	if *SessionCount == 0 {
		*SessionCount = len(cfglist)
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	wg := new(sync.WaitGroup)

	for i := 0; i < *SessionCount; i++ {
		num := i


		if num >= len(cfglist) {
			num %= len(cfglist)
		}

		/*localAddr, err := net.ResolveIPAddr("ip", cfglist[num].destIP)
		if err != nil {
			log.Printf("[%d] error: %s", i, err)
			continue
		}

		LocalBindAddr := &net.TCPAddr{IP: localAddr.IP}*/

		glburl := "http://" + *Address + "/" + cfglist[num].fileName + "?AdaptiveType=HLS"

		theURL, err := url.Parse(glburl)
		if err != nil {
			log.Printf("[%d] error: %s", i, err)
			continue
		}
		wg.Add(1)
		go func(u *url.URL, t int, n int) {

			defer wg.Done()

			httpTransport := &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				Dial:                  make_dialer(false, cfglist[num].destIP),
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				//DisableKeepAlives: true,
			}

			timeout := time.Duration(50 * time.Second)

			// client for glb setup
			client := &http.Client{
				Transport: httpTransport,
				Timeout:   timeout,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			start := time.Now()
			url, err := glbSetup(u, client)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}
			log.Printf("[%d] glb response time: %d ms", n, (int(time.Now().Sub(start)) / 1000000))

			// client for vod setup
			//timeout = time.Duration(5 * time.Second)
			timeout = time.Duration(10 * time.Second)
			client = &http.Client{
				Transport: httpTransport,
				Timeout:   timeout,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			start = time.Now()
			content, url, err := getContent(url, client)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}
			log.Printf("[%d] vod response time: %d ms", n, (int(time.Now().Sub(start)) / 1000000))

			playlist, listType, err := m3u8.DecodeFrom(content, true)
			if err != nil {
				log.Printf("[%d] error: %s", n, err)
				return
			}
			content.Close()

			if listType != m3u8.MEDIA && listType != m3u8.MASTER {
				log.Printf("[%d] error: Not a valid playlist", n)
				return
			}

			if listType == m3u8.MASTER {

				// HLS Live ( OTM Channel )

				masterpl := playlist.(*m3u8.MasterPlaylist)
				for _, variant := range masterpl.Variants {
					if variant != nil {

						msURL, err := absolutize(variant.URI, url)
						if err != nil {
							log.Printf("[%d] error: %s", n, err)
							return
						}
						getPlaylist(msURL, t, client, *Delay)
						break
					}
				}
			} else if listType == m3u8.MEDIA {

				// HLS VOD ( SKYLIFE Prime Movie Pack )

				mediapl := playlist.(*m3u8.MediaPlaylist)

				for _, segment := range mediapl.Segments {
					if segment != nil {
						msURL, err := absolutize(segment.URI, u)
						if err != nil {
							log.Printf("[%d] error: %s", n, err)
							break
						}
						download(msURL, client, segment.Duration, *Delay)
						if *Delay > 0 {
							t -= (*Delay / 1000)
							log.Println("down3 : ", t)
						} else {
							t -= int(segment.Duration)
							log.Println("down4 : ", t)
						}
					} else {
						break
					}

					if t <= 0 {
						break
					}
				}
			}
			log.Printf("[%d] Session End", n)
		}(theURL, *PlayTime, i)
		if *Interval > 0 {
			time.Sleep(time.Duration(*Interval * 1000000))
		}
	}
	wg.Wait()
	log.Println("the all end")
}
