package zObject

type BaseObject struct {
	Id interface{}
}

func (o *BaseObject) GetId() interface{} {
	return o.Id
}

func (o *BaseObject) SetId(id interface{}) {
	o.Id = id
}

type Object interface {
	GetId() interface{}
	SetId(id interface{})
}

type TypedBaseObject[ID comparable] struct {
	Id ID
}

func (o *TypedBaseObject[ID]) GetId() ID {
	return o.Id
}

func (o *TypedBaseObject[ID]) SetId(id ID) {
	o.Id = id
}

type TypedObject[ID comparable] interface {
	GetId() ID
	SetId(id ID)
}
