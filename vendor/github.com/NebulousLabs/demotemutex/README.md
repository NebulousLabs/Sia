demotelock
----------

Package demotemutex provides an extention to sync.Mutex that allows a writelock
to be demoted to a readlock without releasing control to other writelocks.
