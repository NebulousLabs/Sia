Modules Conventions
===================

The modules are a work in progress, not all modules have implemented all
conventions.

All modules have a ModuleNotify method, which returns a channel that receives a
struct{} every time that there is an update to the module. The ModuleNotify
class of methods is primarily used for synchronization, to keep information
consistent between modules, especially during testing.

All modules have a ModuleSubscribe method, which takes a ModuleSubscriber
interface as an argument. The ModuleSubscriber interface contains a single
method, the ReceiveModuleUpdate method, a blocking method used to pass updates
from the parent module. When there is an update, the module will call
ReceiveModuleUpdate on all of its subscribers. To maintain synchronization,
each module should only subscribe to one other module. Updates are guaranteed
to arrive in the correct order. ModuleNotify and ModuleSubscribe should be
defined in subscriber.go ReceiveModuleUpdate (in the child module) should be
described in update.go

All modules should have a persistence.go, which contains the functions for
saving and loading the status of the module.
