#Lock-Bot

We had an issue where multiple people were working on a confluence document,
which doesn't have great support for concurrent edits on the same section. To solve this,
I wrote a silly little bot for Slack which allows people to claim locks on particular section names.
Obviously, this does nothing to actually solve the locking problems with confluence, but
it did give us a way to track what people were working on (assuming they remembered to
actually use the tool).

Lock-bot can be used to lock arbitrary strings, so it could work for anything where a lock
is needed. Similar functionality could definitely be built with hubot plugins, but using
the Slack commands framework was super easy.

##Usage

### Bot
To run the bot, you need to set the following environment variables:

* LIST_LOCK_APP_TOKEN (The slack token for the list locks command)
* LIST_LOCK_COMMAND   (The actual slack command, e.g. `/list-locks`)
* LOCK_APP_TOKEN      (The slack token for the lock command)
* LOCK_COMMAND        (The actual slack command, e.g. `/lock`)
* REDIS_URL           (URL for redis server, e.g. `redis://<u>:<password>@<host>:<port>`)
* UNLOCK_APP_TOKEN    (The slack token for the unlock command)
* UNLOCK_COMMAND      (The actual slack command, e.g. `/unlock`)
* VIEW_LOCK_APP_TOKEN (The slack token for the view lock command)
* VIEW_LOCK_COMMAND   (The actual slack command, e.g. `/view-lock`)

The service exposes 4 endpoints:

* `https://<host>/lock`
* `https://<host>/unlock`
* `https://<host>/listlocks`
* `https://<host>/viewlock`

Each endpoint corresponds to a specific command

### Slack

You will need to create 4 commands in slack to fully use the lock service. The names are
arbitrary, but you will get the idea:

`/lock [item to lock]` -- This tries to create a lock. It will fail if the lock has been acquired by
someone else. It needs to map to `https://<host>/lock`

`/unlock [item to unlock]` -- This tries to unlock an item. You can only unlock items that you
created. It needs to map to `https://<host>/unlock`

`/listlocks` -- This will provide a list of outstanding locks, who owns them, and how long they have
been locked for. It displays privately, so you don't keep notifying people.

`/viewlock [item]` -- This gives information about a specific locked item.

Nothing fancy, but was kinda fun to write and somewhat useful. I do need to add some tests.