Module Conventions
==================

Each module has a file/directory where they store persistent data (if
necessary). When module.New is called, the module is responsible for creating
and populating that directory. The logic for saving and loading data belongs in
persist.go.

Modules that depend on external information (such as the state of consensus)
have an update.go to manage fetching and integrating the external information.
If that information is coming from another module, a subscription should be
used. Module subscription uses a ModuleSubscriber interface (which the
subscriber must satisfy) and a ModuleSubscribe method (implemented by the
parent module). As the parent module gets updates, it will call
ReceiveModuleUpdate (the only method of the ModuleSubscriber interface) on all
subscribers, taking care that each subscriber always receives the updates in
the correct order. This method of subscription is chosen to keep information
flow simple and synchronized - a child module should never have information
that the parent module does not (it just causes problems).

For testing, it is often important to know that an update has propagated to all
modules. Any module that subscribes to another must also implement a
ModuleNotify function in subscriptions.go. ModuleNotify returns a channel down
which a struct{} will be sent every time that module receives an update from a
parent module. To keep things simple, a module should not subscribe to the
parent of another module that it is subscribed to. For example, the transaction
pool is subscribed to the consensus set. Therefore, no module should subscribe
to both the transaction pool and the consensus set. All consensus set updates
should be received through the transaction pool. This helps with
synchronization and ensures that no child module ever has information that the
parent module has not yet received (desynchronization).

#### Module Update Flow

consensus -> (host, hostdb, renter, (transaction pool -> miner, wallet))
