package zEtcd

import (
	"context"
	"fmt"
	"testing"
)

func Test(t *testing.T) {
	cli, err := NewEtcdClient(&ClientConfig{
		Endpoints: []string{"192.168.50.14:2379"},
	})
	cli.SetLogPrint(func(v ...any) {
		fmt.Println(v...)
	})
	/*
		v, err := cli.Get(context.Background(), "/config", false)
		if err != nil {
			return
		}
		fmt.Println(v)

		err = cli.Put(context.Background(), "/abc/ddd/eee", "eee")
		if err != nil {
			fmt.Println(err)
			return
		}
		err = cli.Put(context.Background(), "/abc/ddd/fff", "fff")
		if err != nil {
			fmt.Println(err)
			return
		}

		x, err := cli.Get(context.Background(), "/abc/", true)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(x)
	*/

	/*
		heartbeat := int64(3)

		fmt.Println(time.Now(), "begin")
		_, err = cli.PutWithTTL(context.Background(), "/saver/1", "online", heartbeat*3)
		if err != nil {
			fmt.Println(err)
			return
		}

		go func() {
			//for true {
			time.Sleep(time.Duration(heartbeat) * time.Second)
			cli.PutWithTTL(context.Background(), "/saver/1", time.Now().String(), heartbeat*3)
			//}
		}()

		for i := 0; i < 10; i++ {
			str, err := cli.GetOne(context.Background(), "/saver/1")
			if err != nil {
				fmt.Println(err)
				return
			}

			if str == ""{
				fmt.Println("key not exist")
			}else {
				fmt.Println(time.Now(), str)
			}

			time.Sleep(time.Second * 2)
		}
	*/
	_, err = cli.PutWithTTL(context.Background(), "/saver-1", "ddd", 100)
	if err != nil {
		fmt.Println(err)
		return
	}

	cli.GetOne(context.Background(), "/saver-1")

	/*
		str, err := cli.GetOne(context.Background(), "/config/game_server/1")
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(str)

		watcher, ch := cli.Watch(context.Background(), "/config/game_server/1", false)

		select {
		case e, ok := <-ch:
			if !ok {
				fmt.Println("ddddd")
			}

			switch e.Event {
			case EventCreate:
				log.Println("create", e.Data)
			case EventModify:
				log.Println("modify", e.Data)
			case EventDelete:
				log.Println("delete", e.Data)
			case EventWatchCancel:
				_ = watcher.Close()
				return
			}
		}
	*/

	cli.Delete(context.Background(), "/saver-1")

	fmt.Println(`game over`)
	select {}
}
