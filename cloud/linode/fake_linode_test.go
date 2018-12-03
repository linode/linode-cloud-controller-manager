package linode

import (
	"encoding/json"
	"math/rand"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linode/linodego"
)

type fakeAPI struct {
	t        *testing.T
	instance *linodego.Instance
	ips      []*linodego.InstanceIP
	nb       map[string]*linodego.NodeBalancer
	nbc      map[string]*linodego.NodeBalancerConfig
	nbn      map[string]*linodego.NodeBalancerNode
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
			CreatedStr: "2018-01-01T00:01:01",
			UpdatedStr: "2018-01-01T00:01:01",
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
		nb:  make(map[string]*linodego.NodeBalancer),
		nbc: make(map[string]*linodego.NodeBalancerConfig),
		nbn: make(map[string]*linodego.NodeBalancerNode),
	}
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	urlPath := r.URL.Path
	switch r.Method {
	case "GET":
		whichApi := strings.Split(urlPath[1:], "/")
		switch whichApi[0] {
		case "linode":
			switch whichApi[1] {
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
			rx, _ := regexp.Compile("/nodebalancers/[0-9]+/configs")
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
			rx, _ = regexp.Compile("/nodebalancers/[0-9]+/configs/[0-9]+")
			if rx.MatchString(urlPath) {
				id := filepath.Base(urlPath)
				nbc := f.nbc[id]
				if nbc != nil {
					rr, _ := json.Marshal(nbc)
					_, _ = w.Write(rr)

				}
				return
			}
			rx, _ = regexp.Compile("/nodebalancers/[0-9]+")
			if rx.MatchString(urlPath) {
				id := filepath.Base(urlPath)
				nb := f.nb[id]
				if nb != nil {
					rr, _ := json.Marshal(nb)
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
			nbco := new(linodego.NodeBalancerCreateOptions)
			if err := json.NewDecoder(r.Body).Decode(nbco); err != nil {
				f.t.Fatal(err)
			}
			ip := net.IPv4(byte(rand.Intn(100)), byte(rand.Intn(100)), byte(rand.Intn(100)), byte(rand.Intn(100))).String()
			nb := linodego.NodeBalancer{
				ID:                 rand.Intn(9999),
				Label:              nbco.Label,
				Region:             nbco.Region,
				ClientConnThrottle: *nbco.ClientConnThrottle,
				IPv4:               &ip,

				CreatedStr: time.Now().Format("2006-01-02T15:04:05"),
				UpdatedStr: time.Now().Format("2006-01-02T15:04:05"),
			}
			f.nb[strconv.Itoa(nb.ID)] = &nb
			resp, err := json.Marshal(nb)
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
				SSLCommonName:  "",
				SSLFingerprint: "",
				SSLCert:        nbcco.SSLCert,
				SSLKey:         nbcco.SSLKey,
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
		id := filepath.Base(r.URL.Path)
		delete(f.nb, id)
	case "PUT":
		if strings.Contains(r.URL.Path, "configs") {
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
				SSLCommonName:  "",
				SSLFingerprint: "",
				SSLCert:        nbcco.SSLCert,
				SSLKey:         nbcco.SSLKey,
			}
			f.nbc[strconv.Itoa(nbcc.ID)] = &nbcc
			resp, err := json.Marshal(nbcc)
			if err != nil {
				f.t.Fatal(err)
			}
			_, _ = w.Write(resp)
			return
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
