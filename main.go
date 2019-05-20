package main
import (
	"bytes"
	//"context"
	"crypto/tls"
	"flag"
	"fmt"
	//"image"
	//"image/jpeg"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	//"path"
	"regexp"
	"runtime"
	//"strconv"
	"strings"
	"sync"
	"time"
	"encoding/json"
	//"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/http2"
)
var (
	prod = flag.Bool("prod", false, "Whether to configure itself to be the production server in a container. - do not use yet")
	httpsAddr = flag.String("https_addr", "0.0.0.0:4430", "TLS address to listen on ('host:port' or ':port'). Required.")
	httpAddr  = flag.String("http_addr", "0.0.0.0:8080", "Plain HTTP address to listen on ('host:port', or ':port'). Empty means no HTTP.")
	hostHTTP  = flag.String("http_host", "0.0.0.0:4430", "Optional host or host:port to use for http:// links to this service. By default, this is implied from -http_addr.")
	hostHTTPS = flag.String("https_host", "0.0.0.0:4430", "Optional host or host:port to use for http:// links to this service. By default, this is implied from -https_addr.")
	certs, err = tls.LoadX509KeyPair("server.crt", "server.key")
	amfRoot   = flag.String("amfRoot" ,  "amf.default.svc.cluster.local:4430", "address/domain of AMF")
	ausfRoot  = flag.String("ausfRoot", "ausf.default.svc.cluster.local:4430", "address/domain of AUSF")
	udmRoot   = flag.String("udmRoot" ,  "udm.default.svc.cluster.local:4430", "address/domain of UDM")
	udrRoot   = flag.String("udrRoot" ,  "udr.default.svc.cluster.local:4430", "address/domain of UDR")
	pcfRoot   = flag.String("pcfRoot" ,  "pcf.default.svc.cluster.local:4430", "address/domain of PCF")
)

func main() {
	var srv http.Server
	flag.BoolVar(&http2.VerboseLogs, "verbose", false, "Verbose HTTP/2 debugging.")
	flag.Parse()
	srv.Addr = *httpsAddr
	srv.ConnState = idleTimeoutHook()
	srv.ReadTimeout  = 20 * time.Second
	srv.WriteTimeout = 20 * time.Second
	registerHandlers()
	if *prod {
		*hostHTTP = "server.default.svc.cluster.local"
		*hostHTTPS = "server.default.svc.cluster.local"
		serveProd()
	}
	url := "https://" + httpsHost() + "/"
	log.Printf("Listening on " + url)
	http2.ConfigureServer(&srv, &http2.Server{})
	if *httpAddr != "" {
		go func() {
			log.Printf("Listening on http://" + httpHost() + "/ (for unencrypted HTTP/1)")
			log.Fatal(http.ListenAndServe(*httpAddr, nil))
		}()
	}
	go func() {
		log.Fatal(srv.ListenAndServeTLS("server.crt", "server.key"))
		log.Printf("exiting")
	}()
	select {}
}

func home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		fmt.Printf("not found")
		return
	}
	io.WriteString(w, `<html>
	<body>
	<h1>Go + HTTP/2</h1>
	<p>Welcome to <a href="https://golang.org/">the Go language</a>'s <a
	href="https://http2.github.io/">HTTP/2</a> demo & interop server.</p>
	<p>Congratulations, <b>you're using HTTP/2 right now</b>.</p>
	<p>This server exists for others in the HTTP/2 community to test their HTTP/2 client implementations and point out flaws in our server.</p>
	<p>
	The code is at <a href="https://golang.org/x/net/http2">golang.org/x/net/http2</a> and
	is used transparently by the Go standard library from Go 1.6 and later.
	</p>
	<p>Contact info: <i>bradfitz@golang.org</i>, or <a
	href="https://golang.org/s/http2bug">file a bug</a>.</p>
	<h2>Handlers for testing</h2>
	<ul>
	<li>GET <a href="/reqinfo">/reqinfo</a> to dump the request + headers received</li>
	<li>GET <a href="/clockstream">/clockstream</a> streams the current time every second</li>
	<li>GET <a href="/gophertiles">/gophertiles</a> to see a page with a bunch of images</li>
	<li>GET <a href="/serverpush">/serverpush</a> to see a page with server push</li>
	<li>GET <a href="/file/gopher.png">/file/gopher.png</a> for a small file (does If-Modified-Since, Content-Range, etc)</li>
	<li>GET <a href="/file/go.src.tar.gz">/file/go.src.tar.gz</a> for a larger file (~10 MB)</li>
	<li>GET <a href="/redirect">/redirect</a> to redirect back to / (this page)</li>
	<li>GET <a href="/goroutines">/goroutines</a> to see all active goroutines in this server</li>
	<li>PUT something to <a href="/crc32">/crc32</a> to get a count of number of bytes and its CRC-32</li>
	<li>PUT something to <a href="/ECHO">/ECHO</a> and it will be streamed back to you capitalized</li>
	</ul>
	</body></html>`)
}

func registerHandlers() {
	push := newPushHandler()
	mux := http.NewServeMux()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/serverpush"):
			push.ServeHTTP(w, r) // allow HTTP/2 + HTTP/1.x
			return
			case r.TLS == nil: // do not allow HTTP/1.x for anything else
			http.Redirect(w, r, "https://"+httpsHost()+"/", http.StatusFound)
			return
		}
		if r.ProtoMajor == 1 {
			if r.URL.Path == "/reqinfo" {
				reqInfoHandler(w, r)
				return
			}
			fmt.Printf("not http2")
			return
		}
		mux.ServeHTTP(w, r)
	})
	mux.HandleFunc("/sendstrailers", func(w http.ResponseWriter, req *http.Request) {
		// Before any call to WriteHeader or Write, declare
		// the trailers you will set during the HTTP
		// response. These three headers are actually sent in
		// the trailer.
		w.Header().Set("Trailer", "AtEnd1, AtEnd2")
		w.Header().Add("Trailer", "AtEnd3")

		w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
		w.WriteHeader(http.StatusOK)

		w.Header().Set("AtEnd1", "value 1")
		io.WriteString(w, "This HTTP response has both headers before this text and trailers at the end.\n")
		w.Header().Set("AtEnd2", "value 2")
		w.Header().Set("AtEnd3", "value 3") // These will appear as trailers.
	})
	mux.HandleFunc("/", home)
	mux.HandleFunc("/amfstart", amfstart)
	mux.HandleFunc("/nnrf-disc/v1/nf-instances", Nnrf_NFDiscovery_Request) // not used
	mux.HandleFunc("/nudm-ueau/v1/", Nudm_UEAuthentication_Get_Request)
	mux.HandleFunc("/nudm-uecm/v1/", Nudm_UECM_Registration)
	mux.HandleFunc("/nudm-sdm/v2/", Nudm_SDM_Get)
	mux.HandleFunc("/npcf-ue-policy-control/v1/policies/", Npcf_UEPolicyControl_Create)
	mux.HandleFunc("/notifications/", Npcf_UEPolicyControl_UpdateNotify)
	mux.HandleFunc("/nudr-dr/v2/subscription-data/", Nudr_SubscriptionData)
	mux.HandleFunc("/nausf-auth/v1/ue-authentications",  Nausf_UEAuthentication_Authenticate_Request1) // 8
	mux.HandleFunc("/nausf-auth/v1/ue-authentications/", Nausf_UEAuthentication_Authenticate_Request2) // 21
	mux.HandleFunc("/reqinfo", reqInfoHandler)
	mux.HandleFunc("/ECHO", echoCapitalHandler)
	mux.HandleFunc("/clockstream", clockStreamHandler)
	//mux.Handle("/gophertiles", tiles)
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	})
	stripHomedir := regexp.MustCompile(`/(Users|home)/\w+`)
	mux.HandleFunc("/goroutines", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		buf := make([]byte, 2<<20)
		w.Write(stripHomedir.ReplaceAll(buf[:runtime.Stack(buf, true)], nil))
	})
}

func amfstart(w http.ResponseWriter, r *http.Request) {
	fmt.Println("received start signal")
	// receive start signal, not representative for a UE, as not measured here
	// setup for all connections
	var  amfroot = "https://" +  *amfRoot
	var ausfroot = "https://" + *ausfRoot
	var  udmroot = "https://" +  *udmRoot
	var  pcfroot = "https://" +  *pcfRoot
	var ausf1 = ausfroot + "/nausf-auth/v1/ue-authentications" // 8
	var ausf2 = ausfroot + "/nausf-auth/v1/ue-authentications/authctxid0123456789/eap-session" // 21
	var udm1  =  udmroot + "/nudm-ueau/v1/imsi-012345678901234/registrations/amf-3gpp-access" // 29
	var udm2  =  udmroot + "/nudm-sdm/v2/imsi-012345678901234/am-data" // 32
	var udm3  =  udmroot + "/nudm-sdm/v2/imsi-012345678901234/smf-select-data" // ..
	var udm4  =  udmroot + "/nudm-sdm/v2/imsi-012345678901234/ue-context-in-smf-data" // ..
	var udm5  =  udmroot + "/nudm-sdm/v2/imsi-012345678901234/sdm-subscriptions" // 39a
	var pcf1  =  pcfroot + "/npcf-ue-policy-control/v1/policies" // 46a
	// setup certs
	t := &http2.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{certs},
			InsecureSkipVerify: true,
		},
	}
	c := &http.Client{	Transport: t,	}
	// begin procedure
	{
		// call AUSF 1
		request, _ := http.NewRequest("POST", ausf1, strings.NewReader(`{"servingNetworkName":"5G:mnc000.mcc000.3gppnetwork.org", "supiOrSuci":"suci-0-262-01-1111-0-0-0000000000"}`))
		request.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(request)
		// receive
		if err != nil { fmt.Printf("request error: %v\n",err)	}
		//defer resp.Body.Close()
		content, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("body length:%d\n", len(content))
		//fmt.Println(string(content))
		resp.Body.Close()
	}
	{
		// call AUSF 2
		request, _ := http.NewRequest("POST", ausf2, strings.NewReader(`Content-Type: application/json" -d '{eapPayload: "MDAwMDExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTEx"}`))
		request.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(request)
		// receive
		if err != nil { fmt.Printf("request error: %v\n",err)	}
		//defer resp.Body.Close()
		content, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("body length:%d\n", len(content))
		resp.Body.Close()
	}
	{
		// call UDM 1
		request, _ := http.NewRequest("POST", udm1, strings.NewReader(`{"amfInstanceId": "00000000-0000-0000-0000-000000000000", "deregCallbackUri": "https://127.0.0.1:4430/someamfuri", "guami": {"plmnId": {"mcc": "000", "mnc": "000"}, "amfId": "01abcd"}, "ratType": "NR"}`))
		request.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(request)
		// receive
		if err != nil { fmt.Printf("request error: %v\n",err)	}
		//defer resp.Body.Close()
		content, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("body length:%d\n", len(content))
		resp.Body.Close()
	}
	{
		// call UDM 2
		request, _ := http.NewRequest("GET", udm2, nil)
		resp, err := c.Do(request)
		// receive
		if err != nil { fmt.Printf("request error: %v\n",err)	}
		//defer resp.Body.Close()
		content, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("body length:%d\n", len(content))
		resp.Body.Close()
	}
	{
		// call UDM 3
		request, _ := http.NewRequest("GET", udm3, nil)
		resp, err := c.Do(request)
		// receive
		if err != nil { fmt.Printf("request error: %v\n",err)	}
		//defer resp.Body.Close()
		content, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("body length:%d\n", len(content))
		resp.Body.Close()
	}
	{
		// call UDM 4
		request, _ := http.NewRequest("GET", udm4, nil)
		resp, err := c.Do(request)
		// receive
		if err != nil { fmt.Printf("request error: %v\n",err)	}
		//defer resp.Body.Close()
		content, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("body length:%d\n", len(content))
		resp.Body.Close()
	}
	{
		// call UDM 5
		request, _ := http.NewRequest("POST", udm5, strings.NewReader(`{"nfInstanceId": "0000", "callbackReference": "https://amf.domain.tld/notifications/11111", "monitoredResourceUris": ["https://udm.domain.tld/nudm-ueau/v1/imsi-012345678901234/smf-select-data", "https://udm.domain.tld/nudm-ueau/v1/imsi-012345678901234/am-data", "https://udm.domain.tld/nudm-ueau/v1/imsi-012345678901234/ue-context-in-smf-data"]}`))
		request.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(request)
		// receive
		if err != nil { fmt.Printf("request error: %v\n",err)	}
		//defer resp.Body.Close()
		content, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("body length:%d\n", len(content))
		resp.Body.Close()
	}
	{
		// call PCF 1
		request, _ := http.NewRequest("POST", pcf1, strings.NewReader(`{"notificationUri": "` + amfroot + `/notifications/22222", "supi": "imsi-012345678901234", "suppFeat": "0"}`))
		request.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(request)
		// receive
		if err != nil {
			fmt.Printf("request error: %v\n",err)
		}	else {
			content, _ := ioutil.ReadAll(resp.Body)
			fmt.Printf("body length:%d\n", len(content))
			resp.Body.Close()
		}
	}
	fmt.Printf("sending response\n")
	// registration procedure is over
}

func Npcf_UEPolicyControl_UpdateNotify(w http.ResponseWriter, r *http.Request) {
	// just send a response, assuming the data was correct
	// 200 by default
	// last message, log success here
}

func Npcf_UEPolicyControl_Create(w http.ResponseWriter, r *http.Request) {
	var pcfroot = "https://" + *httpsAddr
	// receive from amf
	fmt.Println(r.URL)
	if r.Method == "POST" {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		jsonString := buf.String()
		fmt.Println("Npcf: %s",jsonString)

		// content type: "application/json"
		type Struct struct {
			Uri string `json:"notificationUri"`
			Supi string `json:"supi"`
			Features string `json:"suppFeat"`
		}

		dec := json.NewDecoder(strings.NewReader(jsonString))
		var s Struct
		for {
			if err := dec.Decode(&s); err == io.EOF {
				break
			} else if err != nil {
				log.Fatal(err)
			}
			if s.Uri != "" && s.Supi != "" && s.Features != "" {
				fmt.Printf("%s , %s, %s\n", s.Uri, s.Supi, s.Features)
				// send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated) // 201
				var resourceUri = pcfroot + r.URL.String() + "/012345"
				w.Header().Set("Location", resourceUri) // policy asso id at end
				io.WriteString(w,`{"suppFeat": "0"}`)

				// send Npcf_UEPolicyControl_UpdateNotify
				var target = s.Uri + "/update" // set target to specified update url
				// call amf
				//certs, err := tls.LoadX509KeyPair("server.crt", "server.key")
				if err != nil {fmt.Printf("error %v\n", err) }
				t := &http2.Transport{
					TLSClientConfig: &tls.Config{
						Certificates: []tls.Certificate{certs},
						InsecureSkipVerify: true,
					},
				}
				c := &http.Client{	Transport: t,	}
				r.Header.Set("Content-Type", "application/json")
				r, _ := http.NewRequest("POST", target, strings.NewReader(`{"resourceUri":"` + resourceUri + `"}`))
				resp, err := c.Do(r)

				// receive AMF 204
				if err != nil { fmt.Printf("request error: %v\n",err)	}
				//defer resp.Body.Close()
				content, _ := ioutil.ReadAll(resp.Body)
				resp.Body.Close()
				fmt.Printf("body length:%d\n", len(content))
				resstring := string(content)
				fmt.Println(resstring)
				if resp.StatusCode == 204 {
					fmt.Println("success chain")
				}
			}
		}
	}
}

func Nudr_SubscriptionData(w http.ResponseWriter, r *http.Request) {
	// receive from udm
	fmt.Println(r.URL)
	{
		re, _ := regexp.Compile("/nudr-dr/v2/subscription-data/(.*)/provisioned-data/am-data")
		values := re.FindStringSubmatch(r.URL.Path)
		if len(values) > 0 { fmt.Println("supi/plmnid: ", values[1]) }
		if r.Method == "GET" && len(values) > 0 {
			// response to udm
			w.Header().Set("Content-Type", "application/json")
			// no required fields in body according to openapi 29.505
		}
		return
	}
	{
		re, _ := regexp.Compile("/nudr-dr/v2/subscription-data/(.*)/provisioned-data/smf-selection-subscription-data")
		values := re.FindStringSubmatch(r.URL.Path)
		if len(values) > 0 { fmt.Println("supi/plmnid: ", values[1]) }
		if r.Method == "GET" && len(values) > 0 {
			// also no body required
			w.Header().Set("Content-Type", "application/json")
		}
		return
	}
	{
		re, _ := regexp.Compile("/nudr-dr/v2/subscription-data/(.*)/context-data/smf-registrations/.*")
		values := re.FindStringSubmatch(r.URL.Path)
		if len(values) > 0 { fmt.Println("supi: ", values[1]) }
		if r.Method == "GET" && len(values) > 0 {
			// return smf context: no context
			w.Header().Set("Content-Type", "application/json")
		}
		return
	}
}

func Nudm_SDM_Get(w http.ResponseWriter, r *http.Request) {
	var udrroot = "https://" + *httpsAddr
	var udmroot = "https://" + *httpsAddr
	{
		re, _ := regexp.Compile("/nudm-sdm/v2/(.*)/am-data")
		values := re.FindStringSubmatch(r.URL.Path)
		if len(values) > 0 { fmt.Println("supi: ", values[1]) }
		if r.Method == "GET" && len(values) > 0 {
			// request has no required data except supi
			// call UDR to get Access and Mobility Subscription data
			var target = udrroot + "/nudr-dr/v2/subscription-data/imsi-012345678901234/111111/provisioned-data/am-data"
			// call UDR with SUPI, PLMNID
			//certs, err := tls.LoadX509KeyPair("server.crt", "server.key")
			if err != nil {fmt.Printf("error %v\n", err) }
			t := &http2.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{certs},
					InsecureSkipVerify: true,
				},
			}
			c := &http.Client{	Transport: t,	Timeout: time.Second * 5 }
			r, err := http.NewRequest("GET", target, nil)
			if err != nil {
				fmt.Println("error gen request: " + err.Error())
				return
			}
			resp, err := c.Do(r)
			// receive AccessAndMobilitySubscriptionData
			if err != nil {
				fmt.Printf("request error: %v\n",err)
				if resp!=nil {
					resp.Body.Close()
					fmt.Println("response nil")
				}
				return
			}
			if resp.StatusCode != 200 {
				fmt.Println("error reading body: " + resp.Status)
				if resp != nil { resp.Body.Close() }
				return
			}
			//defer resp.Body.Close()
			content, _ := ioutil.ReadAll(resp.Body)
			if err != nil {
				fmt.Println("error reading resp body: " + err.Error())
				if resp != nil { resp.Body.Close() }
				return
			}
			resp.Body.Close()
			fmt.Printf("body length:%d\n", len(content))
			resstring := string(content)
			fmt.Println(resstring)

			w.Header().Set("Content-Type", "application/json")
		}
	}
	{
		re, _ := regexp.Compile("/nudm-sdm/v2/(.*)/smf-select-data")
		values := re.FindStringSubmatch(r.URL.Path)
		if len(values) > 0 { fmt.Println("supi: ", values[1]) }
		if r.Method == "GET" && len(values) > 0 {
			// call UDR to get SMF Selection Subscription
			var target = udrroot + "/nudr-dr/v2/subscription-data/imsi-012345678901234/111111/provisioned-data/smf-selection-subscription-data"
			// call UDR with SUPI, PLMNID
			//certs, err := tls.LoadX509KeyPair("server.crt", "server.key")
			if err != nil {fmt.Printf("error %v\n", err) }
			t := &http2.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{certs},
					InsecureSkipVerify: true,
				},
			}
			c := &http.Client{	Transport: t,	}
			r, _ := http.NewRequest("GET", target, nil)
			resp, err := c.Do(r)
			// receive SMF Selection Subscription
			if err != nil { fmt.Printf("request error: %v\n",err)	}
			//defer resp.Body.Close()
			resp.Body.Close()
			content, _ := ioutil.ReadAll(resp.Body)
			fmt.Printf("body length:%d\n", len(content))
			resstring := string(content)
			fmt.Println(resstring)

			w.Header().Set("Content-Type", "application/json")
		}
	}
	{
		re, _ := regexp.Compile("/nudm-sdm/v2/(.*)/ue-context-in-smf-data")
		values := re.FindStringSubmatch(r.URL.Path)
		if len(values) > 0 { fmt.Println("supi: ", values[1]) }
		if r.Method == "GET" && len(values) > 0 {
			// call UDR to get UE context in SMF
			var target = udrroot + "/nudr-dr/v2/subscription-data/imsi-012345678901234/context-data/smf-registrations/111111"
			// call UDR with SUPI, PLMNID
			//certs, err := tls.LoadX509KeyPair("server.crt", "server.key")
			if err != nil {fmt.Printf("error %v\n", err) }
			t := &http2.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{certs},
					InsecureSkipVerify: true,
				},
			}
			c := &http.Client{	Transport: t,	}
			r, _ := http.NewRequest("GET", target, nil)
			resp, err := c.Do(r)
			// receive SMF context
			if err != nil { fmt.Printf("request error: %v\n",err)	}
			//defer resp.Body.Close()
			content, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			fmt.Printf("body length:%d\n", len(content))
			resstring := string(content)
			fmt.Println(resstring)

			// send response to amf
			// no required headers
			// response was empty
			w.Header().Set("Content-Type", "application/json")
		}
	}
	{
		re, _ := regexp.Compile("/nudm-sdm/v2/(.*)/sdm-subscriptions")
		values := re.FindStringSubmatch(r.URL.Path)
		if len(values) > 0 { fmt.Println("supi: ", values[1]) }
		if r.Method == "POST" && len(values) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated) // 201
			w.Header().Set("Location", udmroot + r.URL.String() + "/012345") // subscription id at end
			buf := new(bytes.Buffer)
			buf.ReadFrom(r.Body)
			io.WriteString(w,buf.String())
		}
	}
}

func Nudm_UECM_Registration(w http.ResponseWriter, r *http.Request) {
	var udmroot = *httpsAddr
	// nudm-uecm
	re, _ := regexp.Compile("/nudm-uecm/v1/(.*)/registrations/amf-3gpp-access")
	values := re.FindStringSubmatch(r.URL.Path)
	if len(values) > 0 { fmt.Println("ueid: ", values[1]) } // ueid is supi
	if r.Method == "PUT" && len(values) > 0 {
		// response
		fmt.Println(udmroot + r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated) // 201 setting order matters
		w.Header().Set("Location", udmroot + r.URL.String())
	}

}

func Nausf_UEAuthentication_Authenticate_Request1(w http.ResponseWriter, r *http.Request) {
	// receiving POST auth authentication info
	// {supiOrSuci:"",servingNetworkName:""}
	var ausfroot = "https://" + *ausfRoot
	var udmroot = "https://" + *udmRoot
	var authctxid = "123456789"
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.Body)
	jsonString := buf.String()
	fmt.Println(jsonString)
	type Auth struct {
		Network string `json:"servingNetworkName"`
		Suci string `json:"supiOrSuci"`
	}
	dec := json.NewDecoder(strings.NewReader(jsonString))
	var a Auth
	for {
		if err := dec.Decode(&a); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal("decoding failed")
			log.Fatal(err)
		}
		if a.Network != "" && a.Suci != "" {
			fmt.Printf("%s , %s\n", a.Network, a.Suci)
			//var target = "https://127.0.0.1:4430/nudm-ueau/v1/" + a.Suci + "/security-information/generate-auth-data"
			var target = udmroot + "/nudm-ueau/v1/" + a.Suci + "/security-information/generate-auth-data"
			fmt.Printf("target: %s\n", target)

			// call UDM with SUCI, SN-name
			//certs, err := tls.LoadX509KeyPair("server.crt", "server.key")
			//if err != nil {fmt.Printf("error %v\n", err) }
			t := &http2.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{certs},
					InsecureSkipVerify: true,
				},
			}
			c := &http.Client{	Transport: t,	}
			r, _ := http.NewRequest("POST", target, strings.NewReader(`{"servingNetworkName":"5G:mnc000.mcc000.3gppnetwork.org", "ausfInstanceId":"00000000-0000-0000-0000-000000000000"}`))
			r.Header.Set("Content-Type", "application/json")
			resp, err := c.Do(r)

			// receive AV, SUPI
			fmt.Printf("response: %v\n", resp)
			if err != nil { fmt.Printf("request error\n")	}
			//defer resp.Body.Close()
			content, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			fmt.Printf("body length:%d\n", len(content))
			resstring := string(content)
			fmt.Println(resstring)

			// send response to amf
			w.Header().Set("Content-Type", "application/3gppHal+json")
			w.WriteHeader(http.StatusCreated) // 201 setting order matters
			w.Header().Set("Location", ausfroot + "/nausf-auth/v1/ue-authentications/" + authctxid + "/eap-session")
			// ue auth ctx
			// 5g auth data base64(eappacket)
			io.WriteString(w,"{'authType': 'EAP_AKA_PRIME','5gAuthData': 'MDAwMDExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTEx', '_links':'" + ausfroot + "/nausf-auth/v1/ue-authentications/" + authctxid +  "/eap-session" + "'}")
			fmt.Printf("sending response to amf\n")
		}
	}}

	func Nausf_UEAuthentication_Authenticate_Request2(w http.ResponseWriter, r *http.Request) {
		// receiving POST authctxid and eap-session
		// ue-authentications/{authctxid}/eap-session
		re, _ := regexp.Compile("/ue-authentications/(.*)/eap-session")
		values := re.FindStringSubmatch(r.URL.Path)
		if len(values) > 0 { fmt.Println("authctxid: ", values[1]) } // authctxid is string


		// mac, res

		//response 200 ok eapsession
		// kseaf supi
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w,`{"eapPayload": "", "kSeaf": '0123456789ABCDEF0123456789abcdef0123456789ABCDEF0123456789abcdef', "authResult": "AUTHENTICATION_SUCCESS", "supi": "imsi-012345678901234"}`)
	}

	func Nnrf_NFDiscovery_Request(w http.ResponseWriter, r *http.Request) {
		// up to impl. #registered NF service instances
		w.Header().Set("Content-Type", "text/plain")
		if r.Method == "GET" {
			fmt.Fprintf(w, "Method: %s\n", r.Method)
			// output: set of NF instances: per instance: NF type, NF instance ID, IPs v FQDNs,
			// service instances list (each service instance: service name, NF service instance ID
			// if present in NF profile: Endpoint addresses of NF service instances (may be list of IPs/FQDNs)
			//fmt.Fprintf(w, "") // IP or FQDN of target
		}
		var targetname = r.URL.Query().Get("target-nf-type")
		var requestername = r.URL.Query().Get("requester-nf-type")
		var servicesstring = r.FormValue("service-names")
		var services = strings.Split(servicesstring, ",")
		fmt.Fprintf(w, "%v, %v, %v\n", targetname, requestername, services)
		//fmt.Fprintf(w, "%v", fqdn)

		//23.502p305
		// input: target NF service name, nf type of target nf, nf type of nf service consumer
		// optional: S-NSSAI, NSI ID
		//		DNN, target NF, NF service PLMN ID
		//		NRF to be used to select NFs/services within HPLMN
		//		Serving PLMN ID, NF service consumer ID, preferred target NF location, TAI
		// if target is ausf: optional: Routing ID (part of SUCI), AUSF Group ID

		r.Header.Write(w)
	}

	func Nudm_UEAuthentication_Get_Request(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("URL: %v\n", r.URL)
		fmt.Printf("%v\n", r.URL.Path)
		fmt.Printf("Method: %s\n", r.Method)

		re, _ := regexp.Compile("/nudm-ueau/v1/(.*)")
		values := re.FindStringSubmatch(r.URL.Path)
		if len(values) > 0 {
			r2,_:=regexp.Compile("(.*)/security-information/generate-auth-data")
			values := r2.FindStringSubmatch(values[1])
			if len(values) > 0 {
				fmt.Println("SUCI: ", values[1])
				//{"servingNetworkName":"5G:mnc000.mcc000.3gppnetwork.org", "ausfInstanceId":"00000000-0000-0000-0000-000000000000"}
				buf := new(bytes.Buffer)
				buf.ReadFrom(r.Body)
				jsonString := buf.String()
				fmt.Println("ueau: %s",jsonString)

				// content type: "application/json"
				type Auth struct {
					Network string `json:"servingNetworkName"`
					Instance string `json:"ausfInstanceId"`
				}

				dec := json.NewDecoder(strings.NewReader(jsonString))
				for {
					var a Auth
					if err := dec.Decode(&a); err == io.EOF {
						break
					} else if err != nil {
						log.Fatal(err)
					}
					// required to be present in request as per 29.503
					if a.Network != "" && a.Instance != "" {
						fmt.Printf("%s , %s\n", a.Network, a.Instance)
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w,"{'authType': 'EAP_AKA_PRIME','supi': 'imsi-012345678901234','authenticationVector': {'avType': 'EAP_AKA_PRIME','rand': '0123456789ABCDEF0123456789abcdef','xres': '0123456789ABCDEF0123456789abcdef','autn': '0123456789ABCDEF0123456789abcdef','ckPrime': '0123456789ABCDEF0123456789abcdef','ikPrime': '0123456789ABCDEF0123456789abcdef'}}")
					}
				}

			}
		}
	}

	func reqInfoHandler(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Method: %s\n", r.Method)
		fmt.Fprintf(w, "Protocol: %s\n", r.Proto)
		fmt.Fprintf(w, "Host: %s\n", r.Host)
		fmt.Fprintf(w, "RemoteAddr: %s\n", r.RemoteAddr)
		fmt.Fprintf(w, "RequestURI: %q\n", r.RequestURI)
		fmt.Fprintf(w, "URL: %#v\n", r.URL)
		fmt.Fprintf(w, "Body.ContentLength: %d (-1 means unknown)\n", r.ContentLength)
		fmt.Fprintf(w, "Close: %v (relevant for HTTP/1 only)\n", r.Close)
		fmt.Fprintf(w, "TLS: %#v\n", r.TLS)
		fmt.Fprintf(w, "\nHeaders:\n")
		//r.Header.Write(w)
	}

	type capitalizeReader struct {
		r io.Reader
	}

	func (cr capitalizeReader) Read(p []byte) (n int, err error) {
		n, err = cr.r.Read(p)
		for i, b := range p[:n] {
			if b >= 'a' && b <= 'z' {
				p[i] = b - ('a' - 'A')
			}
		}
		return
	}

	type flushWriter struct {
		w io.Writer
	}

	func (fw flushWriter) Write(p []byte) (n int, err error) {
		n, err = fw.w.Write(p)
		if f, ok := fw.w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}

	func echoCapitalHandler(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			http.Error(w, "PUT required.", 400)
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		io.Copy(flushWriter{w}, capitalizeReader{r.Body})
	}

	func clockStreamHandler(w http.ResponseWriter, r *http.Request) {
		clientGone := w.(http.CloseNotifier).CloseNotify()
		w.Header().Set("Content-Type", "text/plain")
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		fmt.Fprintf(w, "# ~1KB of junk to force browsers to start rendering immediately: \n")
		io.WriteString(w, strings.Repeat("# xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n", 13))
		for {
			fmt.Fprintf(w, "%v\n", time.Now())
			w.(http.Flusher).Flush()
			select {
			case <-ticker.C:
			case <-clientGone:
				log.Printf("Client %v disconnected from the clock", r.RemoteAddr)
				return
			}
		}
	}

	var pushResources = map[string]http.Handler{
	}

	func newPushHandler() http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for path, handler := range pushResources {
				if r.URL.Path == path {
					handler.ServeHTTP(w, r)
					return
				}
			}
			cacheBust := time.Now().UnixNano()
			if pusher, ok := w.(http.Pusher); ok {
				for path := range pushResources {
					url := fmt.Sprintf("%s?%d", path, cacheBust)
					if err := pusher.Push(url, nil); err != nil {
						log.Printf("Failed to push %v: %v", path, err)
					}
				}
			}
			time.Sleep(100 * time.Millisecond) // fake network latency + parsing time
		})
	}

	func httpsHost() string {
		if *hostHTTPS != "" {
			return *hostHTTPS
		}
		if v := *httpsAddr; strings.HasPrefix(v, ":") {
			return "localhost" + v
		} else {
			return v
		}
	}

	func http1Prefix() string {
		if *prod {
			return "https://http1.golang.org"
		}
		return "http://" + httpHost()
	}

	func httpHost() string {
		if *hostHTTP != "" {
			return *hostHTTP
		}
		if v := *httpAddr; strings.HasPrefix(v, ":") {
			return "localhost" + v
		} else {
			return v
		}
	}

	type tcpKeepAliveListener struct {
		*net.TCPListener
	}

	func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
		tc, err := ln.AcceptTCP()
		if err != nil {
			return
		}
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(3 * time.Minute)
		return tc, nil
	}

	func serveProd() {
		log.Printf("running in production mode.")
	}

	const idleTimeout = 5 * time.Minute
	const activeTimeout = 10 * time.Minute

	func idleTimeoutHook() func(net.Conn, http.ConnState) {
		var mu sync.Mutex
		m := map[net.Conn]*time.Timer{}
		return func(c net.Conn, cs http.ConnState) {
			mu.Lock()
			defer mu.Unlock()
			if t, ok := m[c]; ok {
				delete(m, c)
				t.Stop()
			}
			var d time.Duration
			switch cs {
			case http.StateNew, http.StateIdle:
				d = idleTimeout
			case http.StateActive:
				d = activeTimeout
			default:
				return
			}
			m[c] = time.AfterFunc(d, func() {
				log.Printf("closing idle conn %v after %v", c.RemoteAddr(), d)
				go c.Close()
			})
		}
	}
