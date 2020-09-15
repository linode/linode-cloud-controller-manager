package linode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/linode/linodego"
)

type fakeAPI struct {
	t        *testing.T
	instance *linodego.Instance
	ips      []*linodego.InstanceIP
	nb       map[string]*linodego.NodeBalancer
	nbc      map[string]*linodego.NodeBalancerConfig
	nbn      map[string]*linodego.NodeBalancerNode

	requests map[fakeRequest]struct{}
}

type fakeRequest struct {
	Path   string
	Body   string
	Method string
}

type filterStruct struct {
	Label string `json:"label,omitempty"`
	NbID  string `json:"nodebalancer_id,omitempty"`
}

func newFake(t *testing.T) *fakeAPI {
	publicIP := net.ParseIP("45.79.101.25")
	privateIP := net.ParseIP("192.168.133.65")
	instanceName := "test-instance"
	region := "us-east"
	return &fakeAPI{
		t: t,
		instance: &linodego.Instance{
			Label:      instanceName,
			Region:     region,
			Image:      "linode/ubuntu16.04lts",
			Type:       "g6-standard-2",
			Group:      "Linode-Group",
			ID:         123,
			Status:     "running",
			Hypervisor: "kvm",
			IPv4: []*net.IP{
				&publicIP,
				&privateIP,
			},
		},
		ips: []*linodego.InstanceIP{
			{
				Address:  publicIP.String(),
				Public:   true,
				LinodeID: 123,
				Type:     "ipv4",
				Region:   region,
			},
			{
				Address:  privateIP.String(),
				Public:   false,
				LinodeID: 123,
				Type:     "ipv4",
				Region:   region,
			},
		},
		nb:       make(map[string]*linodego.NodeBalancer),
		nbc:      make(map[string]*linodego.NodeBalancerConfig),
		nbn:      make(map[string]*linodego.NodeBalancerNode),
		requests: make(map[fakeRequest]struct{}),
	}
}

func (f *fakeAPI) recordRequest(r *http.Request) {
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	f.requests[fakeRequest{
		Path:   r.URL.Path,
		Method: r.Method,
		Body:   string(bodyBytes),
	}] = struct{}{}
}

func (f *fakeAPI) didRequestOccur(method, path, body string) bool {
	_, ok := f.requests[fakeRequest{
		Path:   path,
		Method: method,
		Body:   body,
	}]
	return ok
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.recordRequest(r)

	w.Header().Set("Content-Type", "application/json")
	urlPath := r.URL.Path
	switch r.Method {
	case "GET":
		whichAPI := strings.Split(urlPath[1:], "/")
		switch whichAPI[0] {
		case "linode":
			switch whichAPI[1] {
			case "instances":
				rx, _ := regexp.Compile("/linode/instances/[0-9]+/ips")
				if rx.MatchString(urlPath) {
					resp := linodego.InstanceIPAddressResponse{
						IPv4: &linodego.InstanceIPv4Response{
							Public:  []*linodego.InstanceIP{f.ips[0]},
							Private: []*linodego.InstanceIP{f.ips[1]},
						},
					}
					rr, _ := json.Marshal(resp)
					_, _ = w.Write(rr)
					return
				}

				rx, _ = regexp.Compile("/linode/instances/[0-9]+")
				if rx.MatchString(urlPath) {
					id := filepath.Base(urlPath)
					if id == strconv.Itoa(f.instance.ID) {
						rr, _ := json.Marshal(&f.instance)
						_, _ = w.Write(rr)
					}
					return
				}

				rx, _ = regexp.Compile("/linode/instances")
				if rx.MatchString(urlPath) {
					res := 0
					data := []linodego.Instance{}
					filter := r.Header.Get("X-Filter")
					if filter == "" {
						data = append(data, *f.instance)
					} else {
						var fs filterStruct
						err := json.Unmarshal([]byte(filter), &fs)
						if err != nil {
							f.t.Fatal(err)
						}
						if fs.Label == f.instance.Label {
							data = append(data, *f.instance)
						}
					}
					resp := linodego.InstancesPagedResponse{
						PageOptions: &linodego.PageOptions{
							Page:    1,
							Pages:   1,
							Results: res,
						},
						Data: data,
					}
					rr, _ := json.Marshal(resp)
					_, _ = w.Write(rr)
					return
				}
			}
		case "nodebalancers":
			rx, _ := regexp.Compile("/nodebalancers/[0-9]+/configs/[0-9]+/nodes/[0-9]+")
			if rx.MatchString(urlPath) {
				id := filepath.Base(urlPath)
				nbn, found := f.nbn[id]
				if found {
					rr, _ := json.Marshal(nbn)
					_, _ = w.Write(rr)

				} else {
					w.WriteHeader(404)
					resp := linodego.APIError{
						Errors: []linodego.APIErrorReason{
							{Reason: "Not Found"},
						},
					}
					rr, _ := json.Marshal(resp)
					_, _ = w.Write(rr)
				}
				return
			}
			rx, _ = regexp.Compile("/nodebalancers/[0-9]+/configs/[0-9]+/nodes")
			if rx.MatchString(urlPath) {
				res := 0
				parts := strings.Split(r.URL.Path[1:], "/")
				nbcID, err := strconv.Atoi(parts[3])
				if err != nil {
					f.t.Fatal(err)
				}

				data := []linodego.NodeBalancerNode{}

				for _, nbn := range f.nbn {
					if nbcID == nbn.ConfigID {
						data = append(data, *nbn)
					}
				}

				resp := linodego.NodeBalancerNodesPagedResponse{
					PageOptions: &linodego.PageOptions{
						Page:    1,
						Pages:   1,
						Results: res,
					},
					Data: data,
				}
				rr, _ := json.Marshal(resp)
				_, _ = w.Write(rr)
				return
			}
			rx, _ = regexp.Compile("/nodebalancers/[0-9]+/configs/[0-9]+")
			if rx.MatchString(urlPath) {
				id := filepath.Base(urlPath)
				nbc, found := f.nbc[id]
				if found {
					rr, _ := json.Marshal(nbc)
					_, _ = w.Write(rr)

				} else {
					w.WriteHeader(404)
					resp := linodego.APIError{
						Errors: []linodego.APIErrorReason{
							{Reason: "Not Found"},
						},
					}
					rr, _ := json.Marshal(resp)
					_, _ = w.Write(rr)
				}
				return
			}
			rx, _ = regexp.Compile("/nodebalancers/[0-9]+/configs")
			if rx.MatchString(urlPath) {
				res := 0
				data := []linodego.NodeBalancerConfig{}
				filter := r.Header.Get("X-Filter")
				if filter == "" {
					for _, n := range f.nbc {
						data = append(data, *n)
					}
				} else {
					var fs filterStruct
					err := json.Unmarshal([]byte(filter), &fs)
					if err != nil {
						f.t.Fatal(err)
					}
					for _, n := range f.nbc {
						if strconv.Itoa(n.NodeBalancerID) == fs.NbID {
							data = append(data, *n)
						}
					}
				}
				resp := linodego.NodeBalancerConfigsPagedResponse{
					PageOptions: &linodego.PageOptions{
						Page:    1,
						Pages:   1,
						Results: res,
					},
					Data: data,
				}
				rr, _ := json.Marshal(resp)
				_, _ = w.Write(rr)
				return
			}
			rx, _ = regexp.Compile("/nodebalancers/[0-9]+")
			if rx.MatchString(urlPath) {
				id := filepath.Base(urlPath)
				nb, found := f.nb[id]
				if found {
					rr, _ := json.Marshal(nb)
					_, _ = w.Write(rr)

				} else {
					w.WriteHeader(404)
					resp := linodego.APIError{
						Errors: []linodego.APIErrorReason{
							{Reason: "Not Found"},
						},
					}
					rr, _ := json.Marshal(resp)
					_, _ = w.Write(rr)
				}
				return
			}
			rx, _ = regexp.Compile("/nodebalancers")
			if rx.MatchString(urlPath) {
				res := 0
				data := []linodego.NodeBalancer{}
				filter := r.Header.Get("X-Filter")
				if filter == "" {
					for _, n := range f.nb {
						data = append(data, *n)
					}
				} else {
					var fs filterStruct
					err := json.Unmarshal([]byte(filter), &fs)
					if err != nil {
						f.t.Fatal(err)
					}
					for _, n := range f.nb {
						if *n.Label == fs.Label {
							data = append(data, *n)
						}
					}
				}
				resp := linodego.NodeBalancersPagedResponse{
					PageOptions: &linodego.PageOptions{
						Page:    1,
						Pages:   1,
						Results: res,
					},
					Data: data,
				}
				rr, _ := json.Marshal(resp)
				_, _ = w.Write(rr)
				return
			}

		}

	case "POST":
		tp := filepath.Base(r.URL.Path)
		if tp == "nodebalancers" {
			nbco := linodego.NodeBalancerCreateOptions{}
			if err := json.NewDecoder(r.Body).Decode(&nbco); err != nil {
				f.t.Fatal(err)
			}

			ip := net.IPv4(byte(rand.Intn(100)), byte(rand.Intn(100)), byte(rand.Intn(100)), byte(rand.Intn(100))).String()
			hostname := fmt.Sprintf("nb-%s.%s.linode.com", strings.Replace(ip, ".", "-", 4), strings.ToLower(nbco.Region))
			nb := linodego.NodeBalancer{
				ID:       rand.Intn(9999),
				Label:    nbco.Label,
				Region:   nbco.Region,
				IPv4:     &ip,
				Hostname: &hostname,
			}

			if nbco.ClientConnThrottle != nil {
				nb.ClientConnThrottle = *nbco.ClientConnThrottle
			}
			f.nb[strconv.Itoa(nb.ID)] = &nb

			for _, nbcco := range nbco.Configs {
				if nbcco.Protocol == "https" {
					if !strings.Contains(nbcco.SSLCert, "BEGIN CERTIFICATE") {
						f.t.Fatal("HTTPS port declared without calid ssl cert", nbcco.SSLCert)
					}
					if !strings.Contains(nbcco.SSLKey, "BEGIN RSA PRIVATE KEY") {
						f.t.Fatal("HTTPS port declared without calid ssl key", nbcco.SSLKey)
					}
				}
				nbc := linodego.NodeBalancerConfig{
					ID:             rand.Intn(9999),
					Port:           nbcco.Port,
					Protocol:       nbcco.Protocol,
					ProxyProtocol:  nbcco.ProxyProtocol,
					Algorithm:      nbcco.Algorithm,
					Stickiness:     nbcco.Stickiness,
					Check:          nbcco.Check,
					CheckInterval:  nbcco.CheckInterval,
					CheckAttempts:  nbcco.CheckAttempts,
					CheckPath:      nbcco.CheckPath,
					CheckBody:      nbcco.CheckBody,
					CheckPassive:   *nbcco.CheckPassive,
					CheckTimeout:   nbcco.CheckTimeout,
					CipherSuite:    nbcco.CipherSuite,
					NodeBalancerID: nb.ID,
					SSLCommonName:  "sslcommonname",
					SSLFingerprint: "sslfingerprint",
					SSLCert:        "<REDACTED>",
					SSLKey:         "<REDACTED>",
				}
				f.nbc[strconv.Itoa(nbc.ID)] = &nbc

				for _, nbnco := range nbcco.Nodes {
					nbn := linodego.NodeBalancerNode{
						ID:             rand.Intn(99999),
						Address:        nbnco.Address,
						Label:          nbnco.Label,
						Weight:         nbnco.Weight,
						Mode:           nbnco.Mode,
						NodeBalancerID: nb.ID,
						ConfigID:       nbc.ID,
					}
					f.nbn[strconv.Itoa(nbn.ID)] = &nbn
				}
			}

			resp, err := json.Marshal(nb)
			if err != nil {
				f.t.Fatal(err)
			}
			_, _ = w.Write(resp)
			return

		} else if tp == "rebuild" {
			parts := strings.Split(r.URL.Path[1:], "/")
			nbcco := new(linodego.NodeBalancerConfigRebuildOptions)
			if err := json.NewDecoder(r.Body).Decode(nbcco); err != nil {
				f.t.Fatal(err)
			}
			nbid, err := strconv.Atoi(parts[1])
			if err != nil {
				f.t.Fatal(err)
			}
			nbcid, err := strconv.Atoi(parts[3])
			if err != nil {
				f.t.Fatal(err)
			}
			if nbcco.Protocol == "https" {
				if !strings.Contains(nbcco.SSLCert, "BEGIN CERTIFICATE") {
					f.t.Fatal("HTTPS port declared without calid ssl cert", nbcco.SSLCert)
				}
				if !strings.Contains(nbcco.SSLKey, "BEGIN RSA PRIVATE KEY") {
					f.t.Fatal("HTTPS port declared without calid ssl key", nbcco.SSLKey)
				}
			}
			nbcc := linodego.NodeBalancerConfig{
				ID:             nbcid,
				Port:           nbcco.Port,
				Protocol:       nbcco.Protocol,
				ProxyProtocol:  nbcco.ProxyProtocol,
				Algorithm:      nbcco.Algorithm,
				Stickiness:     nbcco.Stickiness,
				Check:          nbcco.Check,
				CheckInterval:  nbcco.CheckInterval,
				CheckAttempts:  nbcco.CheckAttempts,
				CheckPath:      nbcco.CheckPath,
				CheckBody:      nbcco.CheckBody,
				CheckPassive:   *nbcco.CheckPassive,
				CheckTimeout:   nbcco.CheckTimeout,
				CipherSuite:    nbcco.CipherSuite,
				NodeBalancerID: nbid,
				SSLCommonName:  "sslcommonname",
				SSLFingerprint: "sslfingerprint",
				SSLCert:        "<REDACTED>",
				SSLKey:         "<REDACTED>",
			}

			f.nbc[strconv.Itoa(nbcc.ID)] = &nbcc
			for k, n := range f.nbn {
				if n.ConfigID == nbcc.ID {
					delete(f.nbn, k)
				}
			}

			for _, n := range nbcco.Nodes {
				node := linodego.NodeBalancerNode{
					ID:             rand.Intn(99999),
					Address:        n.Address,
					Label:          n.Label,
					Weight:         n.Weight,
					Mode:           n.Mode,
					NodeBalancerID: nbid,
					ConfigID:       nbcc.ID,
				}
				f.nbn[strconv.Itoa(node.ID)] = &node
			}
			resp, err := json.Marshal(nbcc)
			if err != nil {
				f.t.Fatal(err)
			}
			_, _ = w.Write(resp)
			return
		} else if tp == "configs" {
			parts := strings.Split(r.URL.Path[1:], "/")
			nbcco := new(linodego.NodeBalancerConfigCreateOptions)
			if err := json.NewDecoder(r.Body).Decode(nbcco); err != nil {
				f.t.Fatal(err)
			}
			nbid, err := strconv.Atoi(parts[1])
			if err != nil {
				f.t.Fatal(err)
			}

			nbcc := linodego.NodeBalancerConfig{
				ID:             rand.Intn(9999),
				Port:           nbcco.Port,
				Protocol:       nbcco.Protocol,
				ProxyProtocol:  nbcco.ProxyProtocol,
				Algorithm:      nbcco.Algorithm,
				Stickiness:     nbcco.Stickiness,
				Check:          nbcco.Check,
				CheckInterval:  nbcco.CheckInterval,
				CheckAttempts:  nbcco.CheckAttempts,
				CheckPath:      nbcco.CheckPath,
				CheckBody:      nbcco.CheckBody,
				CheckPassive:   *nbcco.CheckPassive,
				CheckTimeout:   nbcco.CheckTimeout,
				CipherSuite:    nbcco.CipherSuite,
				NodeBalancerID: nbid,
				SSLCommonName:  "sslcomonname",
				SSLFingerprint: "sslfingerprint",
				SSLCert:        "<REDACTED>",
				SSLKey:         "<REDACTED>",
			}
			f.nbc[strconv.Itoa(nbcc.ID)] = &nbcc

			resp, err := json.Marshal(nbcc)
			if err != nil {
				f.t.Fatal(err)
			}
			_, _ = w.Write(resp)
			return
		} else if tp == "nodes" {
			parts := strings.Split(r.URL.Path[1:], "/")
			nbnco := new(linodego.NodeBalancerNodeCreateOptions)
			if err := json.NewDecoder(r.Body).Decode(nbnco); err != nil {
				f.t.Fatal(err)
			}
			nbid, err := strconv.Atoi(parts[1])
			if err != nil {
				f.t.Fatal(err)
			}
			nbcid, err := strconv.Atoi(parts[3])
			if err != nil {
				f.t.Fatal(err)
			}
			nbn := linodego.NodeBalancerNode{
				ID:             rand.Intn(99999),
				Address:        nbnco.Address,
				Label:          nbnco.Label,
				Status:         "UP",
				Weight:         nbnco.Weight,
				Mode:           nbnco.Mode,
				ConfigID:       nbcid,
				NodeBalancerID: nbid,
			}
			f.nbn[strconv.Itoa(nbn.ID)] = &nbn
			resp, err := json.Marshal(nbn)
			if err != nil {
				f.t.Fatal(err)
			}
			_, _ = w.Write(resp)
			return
		}
	case "DELETE":
		idRaw := filepath.Base(r.URL.Path)
		id, err := strconv.Atoi(idRaw)
		if err != nil {
			f.t.Fatal(err)
		}
		if strings.Contains(r.URL.Path, "nodes") {
			delete(f.nbn, idRaw)
		} else if strings.Contains(r.URL.Path, "configs") {
			delete(f.nbc, idRaw)

			for k, n := range f.nbn {
				if n.ConfigID == id {
					delete(f.nbn, k)
				}
			}
		} else if strings.Contains(r.URL.Path, "nodebalancers") {
			delete(f.nb, idRaw)

			for k, c := range f.nbc {
				if c.NodeBalancerID == id {
					delete(f.nbc, k)
				}
			}

			for k, n := range f.nbn {
				if n.NodeBalancerID == id {
					delete(f.nbn, k)
				}
			}
		}
	case "PUT":
		if strings.Contains(r.URL.Path, "nodes") {
			f.t.Fatal("PUT ...nodes is not supported by the mock API")
		} else if strings.Contains(r.URL.Path, "configs") {
			parts := strings.Split(r.URL.Path[1:], "/")
			nbcco := new(linodego.NodeBalancerConfigUpdateOptions)
			if err := json.NewDecoder(r.Body).Decode(nbcco); err != nil {
				f.t.Fatal(err)
			}
			nbcid, err := strconv.Atoi(parts[3])
			if err != nil {
				f.t.Fatal(err)
			}
			nbid, err := strconv.Atoi(parts[1])
			if err != nil {
				f.t.Fatal(err)
			}

			nbcc := linodego.NodeBalancerConfig{
				ID:             nbcid,
				Port:           nbcco.Port,
				Protocol:       nbcco.Protocol,
				ProxyProtocol:  nbcco.ProxyProtocol,
				Algorithm:      nbcco.Algorithm,
				Stickiness:     nbcco.Stickiness,
				Check:          nbcco.Check,
				CheckInterval:  nbcco.CheckInterval,
				CheckAttempts:  nbcco.CheckAttempts,
				CheckPath:      nbcco.CheckPath,
				CheckBody:      nbcco.CheckBody,
				CheckPassive:   *nbcco.CheckPassive,
				CheckTimeout:   nbcco.CheckTimeout,
				CipherSuite:    nbcco.CipherSuite,
				NodeBalancerID: nbid,
				SSLCommonName:  "sslcommonname",
				SSLFingerprint: "sslfingerprint",
				SSLCert:        "<REDACTED>",
				SSLKey:         "<REDACTED>",
			}
			f.nbc[strconv.Itoa(nbcc.ID)] = &nbcc

			for _, n := range nbcco.Nodes {
				node := linodego.NodeBalancerNode{
					ID:             rand.Intn(99999),
					Address:        n.Address,
					Label:          n.Label,
					Weight:         n.Weight,
					Mode:           n.Mode,
					NodeBalancerID: nbid,
					ConfigID:       nbcc.ID,
				}

				f.nbn[strconv.Itoa(node.ID)] = &node
			}

			resp, err := json.Marshal(nbcc)
			if err != nil {
				f.t.Fatal(err)
			}
			_, _ = w.Write(resp)
			return
		} else if strings.Contains(r.URL.Path, "nodebalancer") {
			parts := strings.Split(r.URL.Path[1:], "/")
			nbuo := new(linodego.NodeBalancerUpdateOptions)
			if err := json.NewDecoder(r.Body).Decode(nbuo); err != nil {
				f.t.Fatal(err)
			}
			if _, err := strconv.Atoi(parts[1]); err != nil {
				f.t.Fatal(err)
			}

			if nb, found := f.nb[parts[1]]; found {
				if nbuo.ClientConnThrottle != nil {
					nb.ClientConnThrottle = *nbuo.ClientConnThrottle
				}
				if nbuo.Label != nil {
					nb.Label = nbuo.Label
				}

				f.nb[strconv.Itoa(nb.ID)] = nb
				resp, err := json.Marshal(nb)
				if err != nil {
					f.t.Fatal(err)
				}
				_, _ = w.Write(resp)
				return
			}

			w.WriteHeader(404)
			resp := linodego.APIError{
				Errors: []linodego.APIErrorReason{
					{Reason: "Not Found"},
				},
			}
			rr, _ := json.Marshal(resp)
			_, _ = w.Write(rr)

		}
	}
}

func randString(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
