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

///////////////////////////////////////////////

type Object interface {
	GetId() interface{}
	SetId(id interface{})
}
