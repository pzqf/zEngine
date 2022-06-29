package zEtcd

import (
	"context"
	"log"
	"sync"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientV3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type LogPrintFunc func(v ...any)

type Client struct {
	cli       *clientV3.Client
	kv        clientV3.KV
	lease     clientV3.Lease
	elections map[string]*concurrency.Election
	sync.RWMutex
	LogPrint LogPrintFunc
}

type ClientConfig struct {
	Endpoints   []string
	UserName    string
	Password    string
	DialTimeout time.Duration
}

type keyValue struct {
	Key   string
	Value string
}

func NewEtcdClient(config *ClientConfig) (*Client, error) {
	c, err := clientV3.New(clientV3.Config{
		Endpoints:   config.Endpoints,
		Username:    config.UserName,
		Password:    config.Password,
		DialTimeout: config.DialTimeout,
	})

	if err != nil {
		return nil, err
	}

	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	_, err = c.Get(ctx, "/config")
	if err != nil {
		log.Println("can't connect the etcd server", config.Endpoints, err)
		return nil, err
	}

	log.Println("etcd server ", config.Endpoints, "connect success")

	return &Client{
		cli:       c,
		kv:        clientV3.NewKV(c),
		lease:     clientV3.NewLease(c),
		elections: make(map[string]*concurrency.Election),
		LogPrint:  log.Println,
	}, nil
}

func (cli *Client) Close() error {
	return cli.cli.Close()
}

func (cli *Client) SetLogPrint(lpf LogPrintFunc) *Client {
	cli.LogPrint = lpf
	return cli
}

func (cli *Client) GetOne(ctx context.Context, key string) (string, error) {
	resp, err := cli.kv.Get(ctx, key)
	if err != nil {
		return "", err
	}

	if len(resp.Kvs) == 0 {
		return "", nil
	}
	return string(resp.Kvs[0].Value), nil
}

func (cli *Client) Get(ctx context.Context, key string, isPrefix bool) ([]keyValue, error) {
	var resp *clientV3.GetResponse
	var err error
	if isPrefix {
		resp, err = cli.kv.Get(ctx, key, clientV3.WithPrefix(), clientV3.WithSort(clientV3.SortByKey, clientV3.SortAscend))
	} else {
		resp, err = cli.kv.Get(ctx, key)
	}

	if err != nil {
		return nil, err
	}

	if len(resp.Kvs) == 0 {
		return nil, nil
	}
	var list []keyValue
	for _, v := range resp.Kvs {

		list = append(list, keyValue{
			Key:   string(v.Key),
			Value: string(v.Value),
		})
	}
	return list, nil
}

func (cli *Client) Delete(ctx context.Context, key string) error {
	_, err := cli.kv.Delete(ctx, key, clientV3.WithPrevKV())
	return err
}

func (cli *Client) Put(ctx context.Context, key, value string) error {
	_, err := cli.kv.Put(ctx, key, value)
	return err
}

// ttl (seconds)

func (cli *Client) PutWithTTL(ctx context.Context, key, value string, ttl int64) (int64, error) {

	leaseResponse, err := cli.lease.Grant(ctx, ttl)
	if err != nil {
		return 0, err
	}
	_, err = cli.kv.Put(ctx, key, value, clientV3.WithLease(leaseResponse.ID))
	return int64(leaseResponse.ID), err
}

func (cli *Client) PutWithNotExist(ctx context.Context, key, value string) error {
	tx := cli.cli.Txn(ctx).If(clientV3.Compare(clientV3.Version(key), "=", 0)).
		Then(clientV3.OpPut(key, value))

	_, err := tx.Commit()
	return err
}

func (cli *Client) PutWithNotExistTTL(ctx context.Context, key, value string, ttl int64) (int64, error) {
	leaseResponse, err := cli.lease.Grant(ctx, ttl)
	if err != nil {
		return 0, err
	}
	_, err = cli.cli.Txn(ctx).If(clientV3.Compare(clientV3.Version(key), "=", 0)).
		Then(clientV3.OpPut(key, value, clientV3.WithLease(leaseResponse.ID))).
		Commit()
	return int64(leaseResponse.ID), nil
}

func (cli *Client) Revoke(ctx context.Context, leaseId int64) error {
	if leaseId <= 0 {
		return nil
	}
	_, err := cli.lease.Revoke(ctx, clientV3.LeaseID(leaseId))
	return err
}

func (cli *Client) KeepAlive(ctx context.Context, key, value string, ttl int64) (int64, error) {
	leaseResponse, err := cli.lease.Grant(ctx, ttl)
	if err != nil {
		return 0, err
	}
	_, err = cli.kv.Put(ctx, key, value, clientV3.WithLease(leaseResponse.ID))
	if err != nil {
		return 0, err
	}

	ch, err := cli.lease.KeepAlive(ctx, leaseResponse.ID)
	if err != nil {
		return 0, err
	}
	go func(key string, ch <-chan *clientV3.LeaseKeepAliveResponse) {
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					cli.LogPrint("the keep alive key:", key, " has closed")
					return
				}
			}
		}
	}(key, ch)

	return int64(leaseResponse.ID), nil
}

func (cli *Client) KeepAliveOnce(ctx context.Context, leaseId int64) error {
	_, err := cli.lease.KeepAliveOnce(ctx, clientV3.LeaseID(leaseId))
	if err != nil {
		return err

	}

	return nil
}

type EventType int

type KeyChangeChan <-chan *WatchEvent

const (
	EventCreate      = EventType(1) //  create event
	EventModify      = EventType(2) //  update event
	EventDelete      = EventType(3) //  delete event
	EventWatchCancel = EventType(4) //  cancel event
	EventChannelSize = 32
)

type WatchEvent struct {
	Event EventType
	Data  keyValue
}

func (cli *Client) Watch(ctx context.Context, key string, isPrefix bool) (clientV3.Watcher, chan *WatchEvent) {
	watcher := clientV3.NewWatcher(cli.cli)
	var watchChan clientV3.WatchChan
	if isPrefix {
		watchChan = watcher.Watch(ctx, key, clientV3.WithPrefix())
	} else {
		watchChan = watcher.Watch(ctx, key)
	}
	watchEventCh := make(chan *WatchEvent, EventChannelSize)

	// start watch
	go func(key string, watchChan clientV3.WatchChan, watchEventCh chan *WatchEvent) {
		for {
			select {
			case ch, ok := <-watchChan:
				if !ok {
					cli.LogPrint("the watch prefix or key:", key, " has cancel")
					watchEventCh <- &WatchEvent{
						Event: EventWatchCancel,
						Data: keyValue{
							Key: key,
						},
					}
					return
				}
				for _, event := range ch.Events {
					c := &WatchEvent{
						Data: keyValue{
							Key:   string(event.Kv.Key),
							Value: "",
						},
					}
					switch event.Type {
					case mvccpb.PUT:
						c.Data.Value = string(event.Kv.Value)
						c.Event = EventCreate
						if event.IsModify() {
							c.Event = EventModify
						}
					case mvccpb.DELETE:
						c.Event = EventDelete
					}
					watchEventCh <- c
				}
			}
		}
	}(key, watchChan, watchEventCh)
	return watcher, watchEventCh
}
