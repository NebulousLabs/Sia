This file documents the rules of consensus in the Sia protocol.

----------------------
-- Block Timestamps --
----------------------

Each block has a minimum allowed timestamp. The minumum timestamp is found by
taking the median timestamp of the previous 11 blocks. If there are not 11
previous blocks, the genesis timestamp is used repeatedly.

------------------
-- Block Target --
------------------
