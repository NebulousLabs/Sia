# SIP 1. Store renter metadata remotely

Status: **draft**.

## Introduction

Currently, `renter` module stores metadata locally.
To recover files, a user needs a copy of the most recent version
of their `renter/` directory. The purpose of this document is to
identify and address everything preventing seed-only recovery.

## Stored metadata

The following renter metadata is stored locally:

  * contracts

    * [contract][FileContract] (1)
    * the private key of the contract
      (see FileContract.UnlockHash field)
    * host network address
    * the most recent [contract revision][FileContractRevision]
    * the list of sector IDs

  * files

    * the filename
    * the file size
    * the location of sectors with data and parity

(1) Contracts are stored in the blockchain as well, but the renter
    needs to know which contracts belong to them.

[FileContract]: https://godoc.org/github.com/NebulousLabs/Sia/types#FileContract
[FileContractRevision]: https://godoc.org/github.com/NebulousLabs/Sia/types#FileContractRevision

## Contracts

Below is the step-by-step analysis of what needs to be done by
the renter to get access to their files.

### Find contracts in the blockchain

The first thing to do is to find contracts in the blockchain.
This can be done by looking through the list of transactions
funded with renter's money.

### Renter private key

The next step is to determine the private key of the renter for
this contract. Currently, renter private keys are generated randomly
and stored locally. Instead private keys should be derived from
the seed as well as the wallet keys. This was proposed in
[issue 2487][issue2487].

[issue2487]: https://github.com/NebulousLabs/Sia/issues/2487

### Find hosts

The next step is to find the host storing the contract.
Currently, there is no official way to do it, but there are several
workarounds relying on [contract-host matching leaks][leaks]. E.g.
ValidProofOutputs and MissedProofOutputs include host unlockhash,
which hosts provide in RPCSettings. A renter can obtain the host
unlockhash from all hosts and build a map from unlockhash to host.
The map can be used to convert a contract to the corresponding host.

Another workaround is to check the contract against all hosts.
After [PR #2304][pr2304] it requires knowledge of the renter private
key (i.e. owning the contract) to check the presence of a contract
on a host. The private key of a contract has been determined already,
see above. This approach is more reliable than using host unlockhashes
(or other unintentional leaks), but it is more expensive as the renter
has to "ask" all hosts to determine which of them holds the contract.

One proper way to do it is to encrypt the host public key with the
renter key (a new symmetric key derived from seed) and to put it to
ArbitraryData field of the transaction creating the contract. When
the renter recovers the contract, it will decrypt the field and
get the public key of the host. Module `hostdb` maps public key to
host network address. This change is backward compatible: currently,
hosts do not care about ArbitraryData field of the transaction
passed to RPCFormContract.

[leaks]: https://github.com/NebulousLabs/Sia/issues/2327
[pr2304]: https://github.com/NebulousLabs/Sia/pull/2304

### The most recent contract revision

Having the contract, the renter private key and the host, the renter
can request the recent contract revision from the host. Although
the RPC doing this [was removed][rm], this can still be done as the
initial step of RPCDownload, RPCReviseContract, and RPCRenewContract.

[rm]: https://github.com/NebulousLabs/Sia/commit/7ef4b72f86c1eb9c66d9412998f8fbdeef6b55eb

The only issue is that hosts can provide outdated contract
revisions. There are multiple ways to resolve this.

  * Keep recent contract revision in the blockchain.
    Expensive but easy.
  * Keep recent contract revision in other hosts and check that
    the host returns the revision greater or equal to ones reported
    by other hosts. It is also expensive because it adds more
    updates to be sent to hosts.
  * Organize upper layers (how we store the data) in such a way that
    we can determine which hosts hold outdated data. Example: store
    the same data on all hosts and make it append-only; the hosts
    with shorter data are outdated and to fix them the renter needs
    to copy the "tail" from the host with longest data.

[pr2303]: https://github.com/NebulousLabs/Sia/pull/2303

### The list of sector IDs

[PR #2584][pr2584] (in review) adds RPCMetadata to host which will
return the list of sector IDs. The maximum length of the list is
limited, so the renter will call this RPC multiple times to get the
full list of sector IDs. Then the renter will check if the IDs match
the Merkle root from the most recent revision.

[pr2584]: https://github.com/NebulousLabs/Sia/pull/2584

## Files

Currently, metadata about files are stored locally.
The rest of the document discusses how it can be stored remotely.

### What is a file

Currently, a file is a non-modifiable contiguous array of bytes.
I want to change this. Instead, a file will be

  * sparse (with the granularity of host sector size, 4M)
  * modifiable in place
  * supporting discards of its parts

These properties will allow using a file as a block device with
internal filesystem (e.g. ext4) or to use it as disk image for VM.

### Format of metadata

A contract will have one or more metadata sectors followed by
data sectors:

| Metadata 1 | Metadata 2 | ... | Metadata N | Data 1 | Data 2 | ... |
| ---------- | ---------- | --- | ---------- | ------ | ------ | --- |

Metadata will be a log of file changes.
I created [proto file](metadata.proto) describing a single change.
Each change will be prefixed with its length encoded as varint.
New changes will be appended to the end of the log.

If the size of metadata if greater than sector size, a new sector is
inserted before the first sector (effectively becoming itself a new
first sector). A record of type MetadataCount is added to the new
sector. When such a record is parsed, the parser gets the total
number of metadata sectors. Thus, in the diagram above **Metadata N**
sector stores the oldest metadata and **Metadata 1** - the youngest.

How to parse metadata:

```
for {
    if end of sector {
        break
    }
    read length
    if length == 0 {
        break
    }
    read change from next length bytes
    if change.type == MetadataCount {
        read metadata sectors from change.count to 1 first
    }
}
```

Sia supports atomic updates of multiple sectors. To change anything,
renter uploads data sectors and corresponding metadata changes in the
same request. Metadata changes are small because they only change
the affected bytes of the first sector.

If a sector of a file is discarded or the whole file is removed, then
the corresponding sector of the host becomes free. When a new sector
is needed on the host, free sectors are used if they are available.
If a sector of a file is overwritten, a new version is written to new
(or free) sector, then parity sectors are written to other hosts,
after that the old sector and old parity sectors become free.
`Write` and `Discard` records have `revision` field that is used
to determine which information about the sector is fresh.

Before renewing a contract, the renter completely removes free sectors
from the contract by sending Delete RPC and rewrites all the metadata.
Metadata rewriting is needed because indices of sectors are shifted
when free data sectors are removed. It is also a good time to skip
unneeded parts of metadata such as writes that were overwritten
subsequently.

Parity sectors are added to a group of sectors stored on different
servers and are uploaded to a non-overlapping set of servers. Metadata
of a parity sector contains corresponding `Write` records of all data
sectors involved. If the host holding the data sector is down,
the renter can recover all data and metadata from parity sectors.

A file is identified with unique integer identifier (e.g. counter).
After a file is removed, its id is never reused.

### Encryption

Thanks to Merkle root and sector IDs, we don't have to authenticate
data: if it is tampered by the host (or when transferred to renter),
the sector hash will be different. So we only need to encrypt it to
keep safe.

A contract has the following types of sectors:

  * metadata
  * data
  * parity
  * free

A free sector does not hold usable data. When it was not free (i.e.
was used for something) it was encrypted and that data is still there.

Sectors are encrypted with a stream cipher with random access (i.e.
which can produce bytes for XOR in the arbitrary location of the
sequence). One contract corresponds to one sequence (i.e. one
encryption key). The cipher has large space for its pseudorandom
sequence. Each sector (metadata, data, and parity) is mapped somewhere
in the sequence. The exact location is determined by hashing metadata
of the sector. For a metadata sectors metadata is the number of that
sector. For data and backup sectors normalized versions of their
records can be used for that purpose.

Metadata is written by appending records to a metadata sector.
The unused end of a metadata sector cannot be encrypted with the same
key (i.e. just have the pseudorandom sequence without any XORed data)
because otherwise, the host can decrypt the subsequent records by
XORing them with what was in the affected bytes before. To avoid this
problem, the renter can either omit filling the unused parts of
metadata sectors at all or put some random data there. In both cases,
it has to ensure having either end-of-sector or a byte decrypted to 0
after the last record (see above for metadata parsing algorithm).
That allows the host to decrypt that one byte when new record
is added, but that byte stores the length of the record which the
host already knows.

Data sectors are overwritten from scratch (without reusing the
pseudorandom sequence from a previous version), so there is no
problem of host decrypting data comparing several versions. However,
it still makes sense to fill unused ends of data sectors with random
bytes not to tell the host the exact size of the data.

### Alternative: write data and metadata interleaved

The downside of the approach described above (keep metadata at the
beginning of the contract) is the need to update two sectors on each
write: the first sector (with metadata) and the data sector. This
results in updating two sectors on the host.

One way to fix this is to write metadata and data in the same sector.
A host sector (4M) will be split into metadata (4K) and data (the
rest). Metadata will be written with repeats to reduce the number
of sectors needed to read the whole metadata. The algorithm of
repeats is as follows:

  * in every sector, write the current metadata (obvious)
  * in every 2nd sector, write last two sectors (current + previous)
  * in every 4th sector, write last 4 sectors
  * in every 8th sector, write last 8 sectors
  * ...

Metadata in sectors except the sectors with full metadata stores a
pointer to the next sector to read metadata from:

| Sector | Has metadata of | `Next` pointer |
| :----: | :-------------: | :------------: |
| 0      | 0               | -              |
| 1      | 0,1             | -              |
| 2      | 2               | 1              |
| 3      | 0,1,2,3         | -              |
| 4      | 4               | 3              |
| 5      | 4,5             | 3              |
| 6      | 6               | 5              |
| 7      | 0,1,2,3,4,5,6,7 | -              |
| 8      | 8               | 7              |
| 9      | 8,9             | 7              |
| 10     | 10              | 9              |

To read the whole metadata, the renter has to read the chain of
sectors of length O(log(N)). (N is the number of sectors.)
Space overhead of this way of storing metadata is also O(log(N)).

If the metadata in the sector is longer than 4K, such a sector
does not have data (metadata takes the whole sector). If the metadata
is longer than 4M, it takes multiple sectors.

To read metadata, the renter has to know where it ends and then read
it up to the beginning, using `next` pointers. The end of metadata
needs to be stored in a sector, the location of which is known to the
renter apriori. (Otherwise, the renter has to update some other sector
to write where is the end now.) Such a location can be the first or
the last sector (or some other fixed index of the sector). Thus, to
reuse a free sector, it must be moved (e.g. to the end). So, to
implement this alternative design, **we need one more change of the
renter-host protocol**: `ModifyAndMove`. It will work as normal
`Modify`, but will also move the sector inside the contract to the
index specified by the renter. It should be cheap for hosts to
implement this.
