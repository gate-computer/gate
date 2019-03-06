// Code generated by internal/cmd/event-types.  DO NOT EDIT.

package event

func (x *FailInternal) EventName() string         { return Event_Type_name[x.EventType()] }
func (x *FailNetwork) EventName() string          { return Event_Type_name[x.EventType()] }
func (x *FailProtocol) EventName() string         { return Event_Type_name[x.EventType()] }
func (x *FailRequest) EventName() string          { return Event_Type_name[x.EventType()] }
func (x *IfaceAccess) EventName() string          { return Event_Type_name[x.EventType()] }
func (x *InstanceConnect) EventName() string      { return Event_Type_name[x.EventType()] }
func (x *InstanceCreateLocal) EventName() string  { return Event_Type_name[x.EventType()] }
func (x *InstanceCreateStream) EventName() string { return Event_Type_name[x.EventType()] }
func (x *InstanceDelete) EventName() string       { return Event_Type_name[x.EventType()] }
func (x *InstanceDisconnect) EventName() string   { return Event_Type_name[x.EventType()] }
func (x *InstanceList) EventName() string         { return Event_Type_name[x.EventType()] }
func (x *InstanceResume) EventName() string       { return Event_Type_name[x.EventType()] }
func (x *InstanceSnapshot) EventName() string     { return Event_Type_name[x.EventType()] }
func (x *InstanceStatus) EventName() string       { return Event_Type_name[x.EventType()] }
func (x *InstanceSuspend) EventName() string      { return Event_Type_name[x.EventType()] }
func (x *InstanceWait) EventName() string         { return Event_Type_name[x.EventType()] }
func (x *ModuleDownload) EventName() string       { return Event_Type_name[x.EventType()] }
func (x *ModuleList) EventName() string           { return Event_Type_name[x.EventType()] }
func (x *ModuleSourceExist) EventName() string    { return Event_Type_name[x.EventType()] }
func (x *ModuleSourceNew) EventName() string      { return Event_Type_name[x.EventType()] }
func (x *ModuleUnref) EventName() string          { return Event_Type_name[x.EventType()] }
func (x *ModuleUploadExist) EventName() string    { return Event_Type_name[x.EventType()] }
func (x *ModuleUploadNew) EventName() string      { return Event_Type_name[x.EventType()] }

func (*FailInternal) EventType() int32         { return int32(Event_FailInternal) }
func (*FailNetwork) EventType() int32          { return int32(Event_FailNetwork) }
func (*FailProtocol) EventType() int32         { return int32(Event_FailProtocol) }
func (*FailRequest) EventType() int32          { return int32(Event_FailRequest) }
func (*IfaceAccess) EventType() int32          { return int32(Event_IfaceAccess) }
func (*InstanceConnect) EventType() int32      { return int32(Event_InstanceConnect) }
func (*InstanceCreateLocal) EventType() int32  { return int32(Event_InstanceCreateLocal) }
func (*InstanceCreateStream) EventType() int32 { return int32(Event_InstanceCreateStream) }
func (*InstanceDelete) EventType() int32       { return int32(Event_InstanceDelete) }
func (*InstanceDisconnect) EventType() int32   { return int32(Event_InstanceDisconnect) }
func (*InstanceList) EventType() int32         { return int32(Event_InstanceList) }
func (*InstanceResume) EventType() int32       { return int32(Event_InstanceResume) }
func (*InstanceSnapshot) EventType() int32     { return int32(Event_InstanceSnapshot) }
func (*InstanceStatus) EventType() int32       { return int32(Event_InstanceStatus) }
func (*InstanceSuspend) EventType() int32      { return int32(Event_InstanceSuspend) }
func (*InstanceWait) EventType() int32         { return int32(Event_InstanceWait) }
func (*ModuleDownload) EventType() int32       { return int32(Event_ModuleDownload) }
func (*ModuleList) EventType() int32           { return int32(Event_ModuleList) }
func (*ModuleSourceExist) EventType() int32    { return int32(Event_ModuleSourceExist) }
func (*ModuleSourceNew) EventType() int32      { return int32(Event_ModuleSourceNew) }
func (*ModuleUnref) EventType() int32          { return int32(Event_ModuleUnref) }
func (*ModuleUploadExist) EventType() int32    { return int32(Event_ModuleUploadExist) }
func (*ModuleUploadNew) EventType() int32      { return int32(Event_ModuleUploadNew) }
