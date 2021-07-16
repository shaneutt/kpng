package ipvssink

import (
	"bytes"
	"log"
	"net"
	"os/exec"
	"time"

	"github.com/spf13/pflag"
	"sigs.k8s.io/kpng/localsink"
	"sigs.k8s.io/kpng/localsink/decoder"
	"sigs.k8s.io/kpng/localsink/filterreset"
	"sigs.k8s.io/kpng/pkg/api/localnetv1"
)

type Backend struct {
	localsink.Config

	dryRun bool

	serviceMap     map[string]*localnetv1.Service
	setServices    []*localnetv1.Service
	deletedService []*localnetv1.Service

	lbs map[string]*ipvsLB
	buf *bytes.Buffer
}

type ipvsLB struct {
	ip        string
	proto     string
	endpoints []string
}

var _ decoder.Interface = &Backend{}

func New() *Backend {
	return &Backend{
		buf:        &bytes.Buffer{},
		serviceMap: map[string]*localnetv1.Service{},
		lbs:        map[string]*ipvsLB{},
	}
}

func (s *Backend) Sink() localsink.Sink {
	return filterreset.New(decoder.New(s))
}

func (s *Backend) BindFlags(flags *pflag.FlagSet) {
	s.Config.BindFlags(flags)

	// real ipvs sink flags
	flags.BoolVar(&s.dryRun, "dry-run", false, "dry run (print instead of applying)")
}

func (s *Backend) Reset() { /* noop, we're wrapped in filterreset */ }

func (s *Backend) Sync() {
	start := time.Now()
	defer log.Print("sync took ", time.Now().Sub(start))

	log.Print("Sync()")

	dummyIface, err := net.InterfaceByName("kube-ipvs0")
	if err != nil {
		exec.Command("ip", "li", "add", "kube-ipvs0", "type", "dummy").Run()
	}

	dummyIface, err = net.InterfaceByName("kube-ipvs0")
	if err != nil {
		log.Fatal("failed to get dummy interface: ", err)
	}

	_ = dummyIface

	addrs, err := dummyIface.Addrs()
	if err != nil {
		log.Fatal("failed to list dummy interface IPs: ", err)
	}

	for _, svc := range s.setServices {
		for _, ip := range svc.IPs.ClusterIPs.V4 {
			gotAddr := false
			for _, addr := range addrs {
				if addr.String() == ip {
					gotAddr = true
					break
				}
			}

			if !gotAddr {
				exec.Command("ip", "a", "add", ip+"/32", "dev", "kube-ipvs0").Run()
			}
		}
	}

	for _, svc := range s.deletedService {
		for _, ip := range svc.IPs.ClusterIPs.V4 {
			exec.Command("ip", "a", "del", ip+"/32", "dev", "kube-ipvs0").Run()
		}
	}

	// TODO

	log.Printf("LBs: %+v", s.lbs)

	s.setServices = s.setServices[:0]
	s.deletedService = s.deletedService[:0]
}

func (s *Backend) SetService(service *localnetv1.Service) {
	log.Printf("SetService(%v)", service)
	// TODO

	key := service.Namespace + "/" + service.Name
	prevSvc := s.serviceMap[key]
	if prevSvc == nil {
		prevSvc = &localnetv1.Service{
			IPs: &localnetv1.ServiceIPs{
				ClusterIPs: localnetv1.NewIPSet(),
			},
		}
	}

	s.serviceMap[key] = service
	s.setServices = append(s.setServices, service)

	for _, newIPv4 := range service.IPs.ClusterIPs.V4 {
		isNew := true
		for _, prevIPv4 := range prevSvc.IPs.ClusterIPs.V4 {
			if prevIPv4 == newIPv4 {
				isNew = false
				break
			}
		}

		if isNew {
			s.lbs[key+":"+newIPv4] = &ipvsLB{
				ip: newIPv4,
			}
		}
	}
}

func (s *Backend) DeleteService(namespace, name string) {
	log.Printf("DeleteService(%q, %q)", namespace, name)
	// TODO

	key := namespace + "/" + name
	svc := s.serviceMap[key]
	delete(s.serviceMap, key)

	s.deletedService = append(s.deletedService, svc)
}

func (s *Backend) SetEndpoint(namespace, serviceName, key string, endpoint *localnetv1.Endpoint) {
	log.Printf("SetEndpoint(%q, %q, %q, %v)", namespace, serviceName, key, endpoint)
	// TODO
}

func (s *Backend) DeleteEndpoint(namespace, serviceName, key string) {
	log.Printf("DeleteEndpoint(%q, %q, %q)", namespace, serviceName, key)
	// TODO
}
