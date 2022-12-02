package zEvent

import (
	"github.com/pzqf/zEngine/zLog"
	"github.com/pzqf/zUtil/zUtils"
)

type EventFunc func(data interface{})

type EventId int32

type Events struct {
	events map[EventId][]EventFunc
}

func (pe *Events) init() {
	pe.events = make(map[EventId][]EventFunc)
}

func (pe *Events) ResisterEvent(eventId EventId, fun EventFunc) {
	pe.events[eventId] = append(pe.events[eventId], fun)
}

func (pe *Events) OnEvent(eventId EventId, data interface{}) {
	funList, ok := pe.events[eventId]
	if !ok {
		return
	}

	for _, fun := range funList {
		go func(fun EventFunc) {
			defer func() {
				err := zUtils.Recover()
				if err != nil {
					zLog.Error(err.Error())
				}
			}()
			fun(data)
		}(fun)
	}
}
