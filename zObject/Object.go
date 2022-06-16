package zObject

type Object struct {
	Id interface{}
}

func (o *Object) GetId() interface{} {
	return o.Id
}
func (o *Object) SetId(id interface{}) {
	o.Id = id
}
