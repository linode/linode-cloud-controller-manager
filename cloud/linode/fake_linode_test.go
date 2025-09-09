package linode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/linode/linodego"
)

const apiVersion = "v4"

type fakeAPI struct {
	t      *testing.T
	nb     map[string]*linodego.NodeBalancer
	nbc    map[string]*linodego.NodeBalancerConfig
	nbn    map[string]*linodego.NodeBalancerNode
	fw     map[int]*linodego.Firewall               // map of firewallID -> firewall
	fwd    map[int]map[int]*linodego.FirewallDevice // map of firewallID -> firewallDeviceID:FirewallDevice
	nbvpcc map[string]*linodego.NodeBalancerVPCConfig
	vpc    map[int]*linodego.VPC
	subnet map[int]*linodego.VPCSubnet

	requests map[fakeRequest]struct{}
	mux      *http.ServeMux
}

type fakeRequest struct {
	Path   string
	Body   string
	Method string
}

func newFake(t *testing.T) *fakeAPI {
	t.Helper()

	fake := &fakeAPI{
		t:        t,
		nb:       make(map[string]*linodego.NodeBalancer),
		nbc:      make(map[string]*linodego.NodeBalancerConfig),
		nbn:      make(map[string]*linodego.NodeBalancerNode),
		fw:       make(map[int]*linodego.Firewall),
		fwd:      make(map[int]map[int]*linodego.FirewallDevice),
		nbvpcc:   make(map[string]*linodego.NodeBalancerVPCConfig),
		vpc:      make(map[int]*linodego.VPC),
		subnet:   make(map[int]*linodego.VPCSubnet),
		requests: make(map[fakeRequest]struct{}),
		mux:      http.NewServeMux(),
	}
	fake.setupRoutes()
	return fake
}

func (f *fakeAPI) ResetRequests() {
	f.requests = make(map[fakeRequest]struct{})
}

func (f *fakeAPI) recordRequest(r *http.Request, urlPath string) {
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	f.requests[fakeRequest{
		Path:   urlPath,
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

// paginatedResponse represents a single response from a paginated
// endpoint.
type paginatedResponse[T any] struct {
	Page    int `json:"page"    url:"page,omitempty"`
	Pages   int `json:"pages"   url:"pages,omitempty"`
	Results int `json:"results" url:"results,omitempty"`
	Data    []T `json:"data"`
}

func (f *fakeAPI) setupRoutes() {
	f.mux.HandleFunc("GET /v4/nodebalancers", func(w http.ResponseWriter, r *http.Request) {
		res := 0
		data := []linodego.NodeBalancer{}
		filter := r.Header.Get("X-Filter")
		if filter == "" {
			for _, n := range f.nb {
				data = append(data, *n)
			}
		} else {
			var fs map[string]string
			err := json.Unmarshal([]byte(filter), &fs)
			if err != nil {
				f.t.Fatal(err)
			}
			for _, n := range f.nb {
				if (n.Label != nil && fs["label"] != "" && *n.Label == fs["label"]) ||
					(fs["ipv4"] != "" && n.IPv4 != nil && *n.IPv4 == fs["ipv4"]) {
					data = append(data, *n)
				}
			}
		}

		resp := paginatedResponse[linodego.NodeBalancer]{
			Page:    1,
			Pages:   1,
			Results: res,
			Data:    data,
		}
		rr, _ := json.Marshal(resp)
		_, _ = w.Write(rr)
	})

	f.mux.HandleFunc("GET /v4/vpcs", func(w http.ResponseWriter, r *http.Request) {
		res := 0
		data := []linodego.VPC{}
		filter := r.Header.Get("X-Filter")
		if filter == "" {
			for _, v := range f.vpc {
				data = append(data, *v)
			}
		} else {
			var fs map[string]string
			err := json.Unmarshal([]byte(filter), &fs)
			if err != nil {
				f.t.Fatal(err)
			}
			for _, v := range f.vpc {
				if v.Label != "" && fs["label"] != "" && v.Label == fs["label"] {
					data = append(data, *v)
				}
			}
		}

		resp := paginatedResponse[linodego.VPC]{
			Page:    1,
			Pages:   1,
			Results: res,
			Data:    data,
		}
		rr, _ := json.Marshal(resp)
		_, _ = w.Write(rr)
	})

	f.mux.HandleFunc("GET /v4/vpcs/{vpcId}/subnets", func(w http.ResponseWriter, r *http.Request) {
		res := 0
		vpcID, err := strconv.Atoi(r.PathValue("vpcId"))
		if err != nil {
			f.t.Fatal(err)
		}

		resp := paginatedResponse[linodego.VPCSubnet]{
			Page:    1,
			Pages:   1,
			Results: res,
			Data:    f.vpc[vpcID].Subnets,
		}
		rr, _ := json.Marshal(resp)
		_, _ = w.Write(rr)
	})

	f.mux.HandleFunc("GET /v4/nodebalancers/{nodeBalancerId}", func(w http.ResponseWriter, r *http.Request) {
		nb, found := f.nb[r.PathValue("nodeBalancerId")]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			resp := linodego.APIError{
				Errors: []linodego.APIErrorReason{
					{Reason: "Not Found"},
				},
			}
			rr, _ := json.Marshal(resp)
			_, _ = w.Write(rr)
			return
		}

		rr, _ := json.Marshal(nb)
		_, _ = w.Write(rr)
	})

	f.mux.HandleFunc("GET /v4/nodebalancers/{nodeBalancerId}/firewalls", func(w http.ResponseWriter, r *http.Request) {
		nodebalancerID, err := strconv.Atoi(r.PathValue("nodeBalancerId"))
		if err != nil {
			f.t.Fatal(err)
		}

		data := paginatedResponse[linodego.Firewall]{
			Page:    1,
			Pages:   1,
			Results: 0,
			Data:    []linodego.Firewall{},
		}

	out:
		for fwid, devices := range f.fwd {
			for _, device := range devices {
				if device.Entity.ID == nodebalancerID {
					data.Data = append(data.Data, *f.fw[fwid])
					data.Results = 1
					break out
				}
			}
		}

		resp, _ := json.Marshal(data)
		_, _ = w.Write(resp)
	})

	// TODO: note that we discard `nodeBalancerId`
	f.mux.HandleFunc("GET /v4/nodebalancers/{nodeBalancerId}/configs", func(w http.ResponseWriter, r *http.Request) {
		res := 0
		data := []linodego.NodeBalancerConfig{}
		filter := r.Header.Get("X-Filter")
		if filter == "" {
			for _, n := range f.nbc {
				data = append(data, *n)
			}
		} else {
			var fs map[string]string
			err := json.Unmarshal([]byte(filter), &fs)
			if err != nil {
				f.t.Fatal(err)
			}
			for _, n := range f.nbc {
				if strconv.Itoa(n.NodeBalancerID) == fs["nodebalancer_id"] {
					data = append(data, *n)
				}
			}
		}
		resp := paginatedResponse[linodego.NodeBalancerConfig]{
			Page:    1,
			Pages:   1,
			Results: res,
			Data:    data,
		}
		rr, err := json.Marshal(resp)
		if err != nil {
			f.t.Fatal(err)
		}
		_, _ = w.Write(rr)
	})

	f.mux.HandleFunc("GET /v4/nodebalancers/{nodeBalancerId}/configs/{configId}/nodes", func(w http.ResponseWriter, r *http.Request) {
		res := 0
		nbcID, err := strconv.Atoi(r.PathValue("configId"))
		if err != nil {
			f.t.Fatal(err)
		}

		data := []linodego.NodeBalancerNode{}

		for _, nbn := range f.nbn {
			if nbcID == nbn.ConfigID {
				data = append(data, *nbn)
			}
		}

		resp := paginatedResponse[linodego.NodeBalancerNode]{
			Page:    1,
			Pages:   1,
			Results: res,
			Data:    data,
		}
		rr, _ := json.Marshal(resp)
		_, _ = w.Write(rr)
	})

	f.mux.HandleFunc("GET /v4/networking/firewalls/{firewallId}/devices", func(w http.ResponseWriter, r *http.Request) {
		fwdId, err := strconv.Atoi(r.PathValue("firewallId"))
		if err != nil {
			f.t.Fatal(err)
		}

		firewallDevices, found := f.fwd[fwdId]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			resp := linodego.APIError{
				Errors: []linodego.APIErrorReason{
					{Reason: "Not Found"},
				},
			}
			rr, _ := json.Marshal(resp)
			_, _ = w.Write(rr)
			return
		}

		firewallDeviceList := []linodego.FirewallDevice{}
		for i := range firewallDevices {
			firewallDeviceList = append(firewallDeviceList, *firewallDevices[i])
		}
		rr, _ := json.Marshal(paginatedResponse[linodego.FirewallDevice]{Page: 1, Pages: 1, Results: len(firewallDeviceList), Data: firewallDeviceList})
		_, _ = w.Write(rr)
	})

	f.mux.HandleFunc("POST /v4/networking/reserved/ips", func(w http.ResponseWriter, r *http.Request) {
		rico := linodego.ReserveIPOptions{}
		if err := json.NewDecoder(r.Body).Decode(&rico); err != nil {
			f.t.Fatal(err)
		}

		ip := net.IPv4(byte(rand.Intn(100)), byte(rand.Intn(100)), byte(rand.Intn(100)), byte(rand.Intn(100))).String()

		rip := linodego.InstanceIP{
			Address:    ip,
			SubnetMask: "32",
			Prefix:     0,
			Type:       linodego.IPTypeIPv4,
			Public:     true,
			RDNS:       "",
			LinodeID:   0,
			Region:     rico.Region,
			VPCNAT1To1: nil,
			Reserved:   true,
		}

		resp, err := json.Marshal(rip)
		if err != nil {
			f.t.Fatal(err)
		}
		_, _ = w.Write(resp)
	})

	f.mux.HandleFunc("POST /v4/nodebalancers", func(w http.ResponseWriter, r *http.Request) {
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
			Tags:     nbco.Tags,
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

		if nbco.FirewallID != 0 {
			createFirewallDevice(nbco.FirewallID, f, linodego.FirewallDeviceCreateOptions{
				ID:   nb.ID,
				Type: "nodebalancer",
			})
		}

		resp, err := json.Marshal(nb)
		if err != nil {
			f.t.Fatal(err)
		}
		_, _ = w.Write(resp)
	})

	f.mux.HandleFunc("POST /v4/nodebalancers/{nodeBalancerId}/configs", func(w http.ResponseWriter, r *http.Request) {
		nbcco := new(linodego.NodeBalancerConfigCreateOptions)
		if err := json.NewDecoder(r.Body).Decode(nbcco); err != nil {
			f.t.Fatal(err)
		}
		nbid, err := strconv.Atoi(r.PathValue("nodeBalancerId"))
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
	})

	f.mux.HandleFunc("POST /v4/nodebalancers/{nodeBalancerId}/configs/{configId}/rebuild", func(w http.ResponseWriter, r *http.Request) {
		nbcco := new(linodego.NodeBalancerConfigRebuildOptions)
		if err := json.NewDecoder(r.Body).Decode(nbcco); err != nil {
			f.t.Fatal(err)
		}
		nbid, err := strconv.Atoi(r.PathValue("nodeBalancerId"))
		if err != nil {
			f.t.Fatal(err)
		}
		nbcid, err := strconv.Atoi(r.PathValue("configId"))
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
	})

	f.mux.HandleFunc("POST /v4/networking/firewalls", func(w http.ResponseWriter, r *http.Request) {
		fco := linodego.FirewallCreateOptions{}
		if err := json.NewDecoder(r.Body).Decode(&fco); err != nil {
			f.t.Fatal(err)
		}

		firewall := linodego.Firewall{
			ID:     rand.Intn(9999),
			Label:  fco.Label,
			Rules:  fco.Rules,
			Tags:   fco.Tags,
			Status: "enabled",
		}

		f.fw[firewall.ID] = &firewall
		resp, err := json.Marshal(firewall)
		if err != nil {
			f.t.Fatal(err)
		}
		_, _ = w.Write(resp)
	})

	f.mux.HandleFunc("POST /v4/vpcs", func(w http.ResponseWriter, r *http.Request) {
		vco := linodego.VPCCreateOptions{}
		if err := json.NewDecoder(r.Body).Decode(&vco); err != nil {
			f.t.Fatal(err)
		}

		subnets := []linodego.VPCSubnet{}
		for _, s := range vco.Subnets {
			subnet := linodego.VPCSubnet{
				ID:    rand.Intn(9999),
				IPv4:  s.IPv4,
				Label: s.Label,
			}
			subnets = append(subnets, subnet)
			f.subnet[subnet.ID] = &subnet
		}
		vpc := linodego.VPC{
			ID:          rand.Intn(9999),
			Label:       vco.Label,
			Description: vco.Description,
			Region:      vco.Region,
			Subnets:     subnets,
		}

		f.vpc[vpc.ID] = &vpc
		resp, err := json.Marshal(vpc)
		if err != nil {
			f.t.Fatal(err)
		}
		_, _ = w.Write(resp)
	})

	f.mux.HandleFunc("DELETE /v4/vpcs/{vpcId}", func(w http.ResponseWriter, r *http.Request) {
		vpcid, err := strconv.Atoi(r.PathValue("vpcId"))
		if err != nil {
			f.t.Fatal(err)
		}

		for k, v := range f.vpc {
			if v.ID == vpcid {
				for _, s := range v.Subnets {
					delete(f.subnet, s.ID)
				}
				delete(f.vpc, k)
			}
		}
	})

	f.mux.HandleFunc("POST /v4/networking/firewalls/{firewallId}/devices", func(w http.ResponseWriter, r *http.Request) {
		fdco := linodego.FirewallDeviceCreateOptions{}
		if err := json.NewDecoder(r.Body).Decode(&fdco); err != nil {
			f.t.Fatal(err)
		}

		firewallID, err := strconv.Atoi(r.PathValue("firewallId"))
		if err != nil {
			f.t.Fatal(err)
		}

		fwd := createFirewallDevice(firewallID, f, fdco)
		resp, err := json.Marshal(fwd)
		if err != nil {
			f.t.Fatal(err)
		}
		_, _ = w.Write(resp)
	})

	f.mux.HandleFunc("DELETE /v4/nodebalancers/{nodeBalancerId}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.nb, r.PathValue("nodeBalancerId"))
		nid, err := strconv.Atoi(r.PathValue("nodeBalancerId"))
		if err != nil {
			f.t.Fatal(err)
		}

		for k, c := range f.nbc {
			if c.NodeBalancerID == nid {
				delete(f.nbc, k)
			}
		}

		for k, n := range f.nbn {
			if n.NodeBalancerID == nid {
				delete(f.nbn, k)
			}
		}
	})

	f.mux.HandleFunc("DELETE /v4/nodebalancers/{nodeBalancerId}/configs/{configId}/nodes/{nodeId}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.nbn, r.PathValue("nodeId"))
	})

	f.mux.HandleFunc("DELETE /v4/nodebalancers/{nodeBalancerId}/configs/{configId}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.nbc, r.PathValue("configId"))

		cid, err := strconv.Atoi(r.PathValue("configId"))
		if err != nil {
			f.t.Fatal(err)
		}

		for k, n := range f.nbn {
			if n.ConfigID == cid {
				delete(f.nbn, k)
			}
		}
	})

	f.mux.HandleFunc("DELETE /v4/networking/firewalls/{firewallId}", func(w http.ResponseWriter, r *http.Request) {
		firewallId, err := strconv.Atoi(r.PathValue("firewallId"))
		if err != nil {
			f.t.Fatal(err)
		}

		delete(f.fwd, firewallId)
		delete(f.fw, firewallId)
	})

	f.mux.HandleFunc("DELETE /v4/networking/firewalls/{firewallId}/devices/{deviceId}", func(w http.ResponseWriter, r *http.Request) {
		firewallId, err := strconv.Atoi(r.PathValue("firewallId"))
		if err != nil {
			f.t.Fatal(err)
		}

		deviceId, err := strconv.Atoi(r.PathValue("deviceId"))
		if err != nil {
			f.t.Fatal(err)
		}
		delete(f.fwd[firewallId], deviceId)
	})

	// TODO: reimplement all of this
	f.mux.HandleFunc("PUT /v4/nodebalancers/{nodeBalancerId}/configs/{configId}", func(w http.ResponseWriter, r *http.Request) {
		nbcco := new(linodego.NodeBalancerConfigUpdateOptions)
		if err := json.NewDecoder(r.Body).Decode(nbcco); err != nil {
			f.t.Fatal(err)
		}
		nbcid, err := strconv.Atoi(r.PathValue("configId"))
		if err != nil {
			f.t.Fatal(err)
		}
		nbid, err := strconv.Atoi(r.PathValue("nodeBalancerId"))
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
	})

	f.mux.HandleFunc("PUT /v4/networking/firewalls/{firewallID}/rules", func(w http.ResponseWriter, r *http.Request) {
		fwrs := new(linodego.FirewallRuleSet)
		if err := json.NewDecoder(r.Body).Decode(fwrs); err != nil {
			f.t.Fatal(err)
		}

		fwID, err := strconv.Atoi(r.PathValue("firewallID"))
		if err != nil {
			f.t.Fatal(err)
		}

		if firewall, found := f.fw[fwID]; found {
			firewall.Rules.Inbound = fwrs.Inbound
			firewall.Rules.InboundPolicy = fwrs.InboundPolicy
			// outbound rules do not apply, ignoring.
			f.fw[fwID] = firewall
			resp, err := json.Marshal(firewall)
			if err != nil {
				f.t.Fatal(err)
			}
			_, _ = w.Write(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
		resp := linodego.APIError{
			Errors: []linodego.APIErrorReason{
				{Reason: "Not Found"},
			},
		}
		rr, _ := json.Marshal(resp)
		_, _ = w.Write(rr)
	})

	f.mux.HandleFunc("PUT /v4/nodebalancers/{nodeBalancerId}", func(w http.ResponseWriter, r *http.Request) {
		nbuo := new(linodego.NodeBalancerUpdateOptions)
		if err := json.NewDecoder(r.Body).Decode(nbuo); err != nil {
			f.t.Fatal(err)
		}
		if _, err := strconv.Atoi(r.PathValue("nodeBalancerId")); err != nil {
			f.t.Fatal(err)
		}

		if nb, found := f.nb[r.PathValue("nodeBalancerId")]; found {
			if nbuo.ClientConnThrottle != nil {
				nb.ClientConnThrottle = *nbuo.ClientConnThrottle
			}
			if nbuo.Label != nil {
				nb.Label = nbuo.Label
			}
			if nbuo.Tags != nil {
				nb.Tags = *nbuo.Tags
			}

			f.nb[strconv.Itoa(nb.ID)] = nb
			resp, err := json.Marshal(nb)
			if err != nil {
				f.t.Fatal(err)
			}
			_, _ = w.Write(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
		resp := linodego.APIError{
			Errors: []linodego.APIErrorReason{
				{Reason: "Not Found"},
			},
		}
		rr, _ := json.Marshal(resp)
		_, _ = w.Write(rr)
	})
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("fakeAPI: %s %s", r.Method, r.URL.Path)

	urlPath := strings.TrimPrefix(r.URL.Path, "/"+apiVersion)
	f.recordRequest(r, urlPath)

	w.Header().Add("Content-Type", "application/json")
	f.mux.ServeHTTP(w, r)
}

func createFirewallDevice(fwId int, f *fakeAPI, fdco linodego.FirewallDeviceCreateOptions) linodego.FirewallDevice {
	fwd := linodego.FirewallDevice{
		ID: fdco.ID,
		Entity: linodego.FirewallDeviceEntity{
			ID:   fdco.ID,
			Type: fdco.Type,
		},
	}

	if _, ok := f.fwd[fwId]; !ok {
		f.fwd[fwId] = make(map[int]*linodego.FirewallDevice)
	}
	f.fwd[fwId][fwd.ID] = &fwd
	return fwd
}

func randString() string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 10)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
