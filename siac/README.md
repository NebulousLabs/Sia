Siac Usage
==========

siac is the command line interface to Sia, for use by power users and
those on headless servers.

Common tasks
------------

#### Wallet tasks

`siac wallet init [-p]` encrypts and initializes the wallet. If the
`-p` flag is provided, an encryption password is requested from the
user. Otherwise the initial seed is used as the encryption password.

Examples:
```bash
triazo@copper:~$ siac -a :9920 wallet init
Seed is:
 cider sailor incur sober feast unhappy mundane sadness hinder aglow imitate amaze duties arrow gigantic uttered inflamed girth myriad jittery hexagon nail lush reef sushi pastry southern inkling acquire

Wallet encrypted with password: cider sailor incur sober feast unhappy mundane sadness hinder aglow imitate amaze duties arrow gigantic uttered inflamed girth myriad jittery hexagon nail lush reef sushi pastry southern inkling acquire
```

```bash
triazo@copper:~$ siac -a :9920 wallet init -p
Wallet password:
Seed is:
 potato haunted fuming lordship library vane fever powder zippers fabrics dexterity hoisting emails pebbles each vampire rockets irony summon sailor lemon vipers foxes oneself glide cylinder vehicle mews acoustic

Wallet encrypted with given password
```
