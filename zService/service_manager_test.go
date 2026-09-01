package zService

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type serviceCallLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *serviceCallLog) add(call string) {
	l.mu.Lock()
	l.calls = append(l.calls, call)
	l.mu.Unlock()
}

func (l *serviceCallLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

type managerTestService struct {
	*BaseService
	log        *serviceCallLog
	initErr    error
	closeErr   error
	servePanic interface{}
}

func newManagerTestService(id string, log *serviceCallLog) *managerTestService {
	return &managerTestService{BaseService: NewBaseService(id), log: log}
}

func (s *managerTestService) Init() error {
	s.log.add("init:" + s.ServiceId())
	return s.initErr
}

func (s *managerTestService) Close() error {
	s.log.add("close:" + s.ServiceId())
	return s.closeErr
}

func (s *managerTestService) Serve() {
	s.log.add("serve:" + s.ServiceId())
	if s.servePanic != nil {
		panic(s.servePanic)
	}
}

func TestTopologicalSortRejectsMissingAndCircularDependencies(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		manager := NewServiceManager()
		if err := manager.AddService(newManagerTestService("api", &serviceCallLog{}), "store"); err != nil {
			t.Fatal(err)
		}
		if err := manager.InitServices(); !errors.Is(err, ErrServiceDependencyMissing) {
			t.Fatalf("InitServices() error = %v, want %v", err, ErrServiceDependencyMissing)
		}
	})

	t.Run("cycle", func(t *testing.T) {
		manager := NewServiceManager()
		log := &serviceCallLog{}
		if err := manager.AddService(newManagerTestService("a", log), "b"); err != nil {
			t.Fatal(err)
		}
		if err := manager.AddService(newManagerTestService("b", log), "a"); err != nil {
			t.Fatal(err)
		}
		if err := manager.InitServices(); !errors.Is(err, ErrServiceDependencyCycle) {
			t.Fatalf("InitServices() error = %v, want %v", err, ErrServiceDependencyCycle)
		}
	})
}

func TestTopologicalOrderIsDeterministicAndDependencyFirst(t *testing.T) {
	var baseline []string
	for run := 0; run < 30; run++ {
		manager := NewServiceManager()
		log := &serviceCallLog{}
		services := []*managerTestService{
			newManagerTestService("world", log),
			newManagerTestService("api", log),
			newManagerTestService("store", log),
			newManagerTestService("audit", log),
		}
		for _, service := range services {
			var deps []interface{}
			switch service.ServiceId() {
			case "world", "api":
				deps = []interface{}{"store"}
			}
			if err := manager.AddService(service, deps...); err != nil {
				t.Fatal(err)
			}
		}
		if err := manager.InitServices(); err != nil {
			t.Fatal(err)
		}
		got := log.snapshot()
		if indexOf(got, "init:store") > indexOf(got, "init:api") ||
			indexOf(got, "init:store") > indexOf(got, "init:world") {
			t.Fatalf("dependency initialized after dependent: %v", got)
		}
		if run == 0 {
			baseline = got
		} else if fmt.Sprint(got) != fmt.Sprint(baseline) {
			t.Fatalf("run %d order = %v, want %v", run, got, baseline)
		}
	}
}

func TestInitFailureRollsBackInitializedServicesInReverse(t *testing.T) {
	initErr := errors.New("init b failed")
	rollbackErr := errors.New("close a failed")
	log := &serviceCallLog{}
	manager := NewServiceManager()
	a := newManagerTestService("a", log)
	a.closeErr = rollbackErr
	b := newManagerTestService("b", log)
	b.initErr = initErr
	if err := manager.AddService(a); err != nil {
		t.Fatal(err)
	}
	if err := manager.AddService(b, "a"); err != nil {
		t.Fatal(err)
	}

	err := manager.InitServices()
	if !errors.Is(err, initErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("InitServices() error = %v, want init and rollback errors", err)
	}
	want := []string{"init:a", "init:b", "close:a"}
	if got := log.snapshot(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if a.GetState() != ServiceStateStopped || b.GetState() != ServiceStateCreated {
		t.Fatalf("states = a:%v b:%v, want stopped/created", a.GetState(), b.GetState())
	}
}

func TestCloseServicesRunsAllInReverseAndAggregatesErrors(t *testing.T) {
	errB := errors.New("close b failed")
	errC := errors.New("close c failed")
	log := &serviceCallLog{}
	manager := NewServiceManager()
	a := newManagerTestService("a", log)
	b := newManagerTestService("b", log)
	c := newManagerTestService("c", log)
	b.closeErr = errB
	c.closeErr = errC
	if err := manager.AddService(a); err != nil {
		t.Fatal(err)
	}
	if err := manager.AddService(b, "a"); err != nil {
		t.Fatal(err)
	}
	if err := manager.AddService(c, "b"); err != nil {
		t.Fatal(err)
	}
	if err := manager.InitServices(); err != nil {
		t.Fatal(err)
	}
	for _, service := range []*managerTestService{a, b, c} {
		service.SetState(ServiceStateRunning)
	}

	err := manager.CloseServices()
	if !errors.Is(err, errB) || !errors.Is(err, errC) {
		t.Fatalf("CloseServices() error = %v, want both close errors", err)
	}
	wantClose := []string{"close:c", "close:b", "close:a"}
	got := log.snapshot()
	got = got[len(got)-len(wantClose):]
	if fmt.Sprint(got) != fmt.Sprint(wantClose) {
		t.Fatalf("close order = %v, want %v", got, wantClose)
	}
	for _, service := range []*managerTestService{a, b, c} {
		if service.GetState() != ServiceStateStopped {
			t.Fatalf("service %s state = %v, want stopped", service.ServiceId(), service.GetState())
		}
	}
}

func TestServeReturnAndPanicBecomeStopped(t *testing.T) {
	for _, panicValue := range []interface{}{nil, "boom"} {
		name := "return"
		if panicValue != nil {
			name = "panic"
		}
		t.Run(name, func(t *testing.T) {
			manager := NewServiceManager()
			service := newManagerTestService("service", &serviceCallLog{})
			service.servePanic = panicValue
			if err := manager.AddService(service); err != nil {
				t.Fatal(err)
			}
			if err := manager.InitServices(); err != nil {
				t.Fatal(err)
			}
			if err := manager.ServeServicesChecked(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for service.GetState() != ServiceStateStopped && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if service.GetState() != ServiceStateStopped {
				t.Fatalf("state = %v, want stopped", service.GetState())
			}
		})
	}
}

func indexOf(values []string, value string) int {
	for i, candidate := range values {
		if candidate == value {
			return i
		}
	}
	return len(values)
}
