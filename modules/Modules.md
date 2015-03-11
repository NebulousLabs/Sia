Modules Conventions
===================

The modules are a work in progress, not all modules have implemented all
conventions.

All modules should have an update.go which contains the functions for
integrating changes in the environment into the module (for example, changes in
the consensus set).

All modules should have a persistence.go, which contains the functions for
saving and loading the status of the module.
