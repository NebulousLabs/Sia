Sia 0.1.0

This distribution is an early beta release. It is likely to have many bugs,
some of which may be severe. Please use with caution.

This release comes with 2 binaries, siad and siac. This README only covers
siad. Siad uses the files in the 'style/' folder to build webpages that yon can
use to interact with siad. Siad will look for these files in your home folder,
in '$home/.config/sia/style'. Please copy the style/ folder into
'$home/.config/sia/style'. You can also run 'siad install $path/style/', where
$path is the current directory.

To use siad, run the executable in the command prompt (drag and drop the
executable into the command prompt and hit enter). If you haven't put the
style/ folder in the right spot, the program will fail and spit out an error.
If you have, then the program will appear to do nothing. Open up a web browser
and go to 'localhost:9980'. You should see the webpage which is the sia front
end.

In this release, there's no way to save or load private keys. If you turn off
your system, you'll lose all of your coins and files, and you'll lose everyone
else's files. Don't feel bad though, beta coins are worthless anyway.

You can upload files to existing hosts, and you can become a host yourself. The
host process is currently a bit unintuitive. When you become a host, you have
to put up coins that say "I will not lose files", and if you do lose files,
then you lose the coins. So initially, as people make contracts with you, your
balance will actually go down. Then, as you successfully submit storage proofs,
your balance will go back up, until you have made a profit instead of a loss.


When you announce as a host, you freeze some coins, which minimizes spam. Hosts
which freeze few coins are going to be ignored by clients, and hosts which
freeze many coins are going to be favored. There are 3 ways to be a favored
host: lower your price, increase your penalty, and increase the number of coins
you freeze. If you freeze your coins for 100 blocks, you can spend them again
once 100 blocks have passed. Except, right now the host forgets about the
frozen coins (like a squirrel who forgot about a nut it buried), so you can't
actually spend your frozen coins ever again.

Thanks for being a beta user!

Upcoming features:
	+ Save and load wallets
	+ Remeber files you've previously uploaded
	+ Hosts remember coins they've frozen so they can be spent again

Please tell me about any problems you run into, and any features you want! The
advantage of being a beta user is that your feedback will have a large impact
on what we do in the next few months. And thanks again.
