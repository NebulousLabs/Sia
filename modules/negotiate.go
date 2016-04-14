package modules

import (
	"bytes"
	"errors"
	"io"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// AcceptResponse is the response given to an RPC call to indicate
	// acceptance. (Any other string indicates rejection, and describes the
	// reason for rejection.)
	AcceptResponse = "accept"

	// NegotiateFileContractTime defines the amount of time that the renter and
	// host have to negotiate a file contract. The time is set high enough that
	// a node behind Tor has a reasonable chance at making the multiple
	// required round trips to complete the negotiation.
	NegotiateFileContractTime = 360 * time.Second

	// NegotiateFileContractRevisionTime defines the minimum amount of time
	// that the renter and host have to negotiate a file contract revision. The
	// time is set high enough that a full 4MB can be piped through a
	// connection that is running over Tor.
	NegotiateFileContractRevisionTime = 600 * time.Second

	// NegotiateSettingsTime establishes the minimum amount of time that the
	// connection deadline is expected to be set to when settings are being
	// requested from the host. The deadline is long enough that the connection
	// should be successful even if both parties are on Tor.
	NegotiateSettingsTime = 120 * time.Second

	// MaxErrorSize indicates the maximum number of bytes that can be used to
	// encode an error being sent during negotiation.
	MaxErrorSize = 256

	// MaxFileContractSetLen determines the maximum allowed size of a
	// transaction set that can be sent when trying to negotiate a file
	// contract. The transaction set will contain all of the unconfirmed
	// dependencies of the file contract, meaning that it can be quite large.
	// The transaction pool's size limit for transaction sets has been chosen
	// as a reasonable guideline for determining what is too large.
	MaxFileContractSetLen = TransactionSetSizeLimit - 1e3

	// MaxHostExternalSettingsLen is the maximum allowed size of an encoded
	// HostExternalSettings.
	MaxHostExternalSettingsLen = 16000
)

var (
	// ActionDelete is the specifier for a RevisionAction that deletes a
	// sector.
	ActionDelete = types.Specifier{'D', 'e', 'l', 'e', 't', 'e'}

	// ActionInsert is the specifier for a RevisionAction that inserts a
	// sector.
	ActionInsert = types.Specifier{'I', 'n', 's', 'e', 'r', 't'}

	// ActionModify is the specifier for a RevisionAction that modifies sector
	// data.
	ActionModify = types.Specifier{'M', 'o', 'd', 'i', 'f', 'y'}

	// ErrAnnNotAnnouncement indicates that the provided host announcement does
	// not use a recognized specifier, indicating that it's either not a host
	// announcement or it's not a recognized version of a host announcement.
	ErrAnnNotAnnouncement = errors.New("provided data does not form a recognized host announcement")

	// ErrAnnUnrecognizedSignature is returned when the signature in a host
	// announcement is not a type of signature that is recognized.
	ErrAnnUnrecognizedSignature = errors.New("the signature provided in the host announcement is not recognized")

	// PrefixHostAnnouncement is used to indicate that a transaction's
	// Arbitrary Data field contains a host announcement. The encoded
	// announcement will follow this prefix.
	PrefixHostAnnouncement = types.Specifier{'H', 'o', 's', 't', 'A', 'n', 'n', 'o', 'u', 'n', 'c', 'e', 'm', 'e', 'n', '2'}

	// RPCSettings is the specifier for requesting settings from the host.
	RPCSettings = types.Specifier{'S', 'e', 't', 't', 'i', 'n', 'g', 's', 2}

	// RPCFormContract is the specifier for forming a contract with a host.
	RPCFormContract = types.Specifier{'F', 'o', 'r', 'm', 'C', 'o', 'n', 't', 'r', 'a', 'c', 't', 2}

	// RPCRenew is the specifier to renewing an existing contract.
	RPCRenew = types.Specifier{'R', 'e', 'n', 'e', 'w', 2}

	// RPCReviseContract is the specifier for revising an existing file
	// contract.
	RPCReviseContract = types.Specifier{'R', 'e', 'v', 'i', 's', 'e', 'C', 'o', 'n', 't', 'r', 'a', 'c', 't', 2}

	// RPCDownload is the specifier for downloading a file from a host.
	RPCDownload = types.Specifier{'D', 'o', 'w', 'n', 'l', 'o', 'a', 'd', 2}

	// SectorSize defines how large a sector should be in bytes. The sector
	// size needs to be a power of two to be compatible with package
	// merkletree. 4MB has been chosen for the live network because large
	// sectors significantly reduce the tracking overhead experienced by the
	// renter and the host.
	SectorSize = func() uint64 {
		if build.Release == "dev" {
			return 1 << 20 // 1 MiB
		}
		if build.Release == "standard" {
			return 1 << 22 // 4 MiB
		}
		if build.Release == "testing" {
			return 1 << 12 // 4 KiB
		}
		panic("unrecognized release constant in host - sectorSize")
	}()
)

type (
	// HostAnnouncement is an announcement by the host that appears in the
	// blockchain. 'Specifier' is always 'PrefixHostAnnouncement'. The
	// announcement is always followed by a signature from the public key of
	// the whole announcement.
	HostAnnouncement struct {
		Specifier  types.Specifier
		NetAddress NetAddress
		PublicKey  types.SiaPublicKey
	}

	// HostExternalSettings are the parameters advertised by the host. These
	// are the values that the renter will request from the host in order to
	// build its database.
	HostExternalSettings struct {
		// MaxBatchSize indicates the maximum size in bytes that a batch is
		// allowed to be. A batch is an array of revision actions, each
		// revision action can have a different number of bytes, depending on
		// the action, so the number of revision actions allowed depends on the
		// sizes of each.
		AcceptingContracts bool              `json:"acceptingcontracts"`
		MaxBatchSize       uint64            `json:"maxbatchsize"`
		MaxDuration        types.BlockHeight `json:"maxduration"`
		NetAddress         NetAddress        `json:"netaddress"`
		RemainingStorage   uint64            `json:"remainingstorage"`
		SectorSize         uint64            `json:"sectorsize"`
		TotalStorage       uint64            `json:"totalstorage"`
		UnlockHash         types.UnlockHash  `json:"unlockhash"`
		WindowSize         types.BlockHeight `json:"windowsize"`

		// Collateral is the amount of collateral that the host will put up for
		// storage in 'bytes per block', as an assurance to the renter that the
		// host really is committed to keeping the file. But, because the file
		// contract is created with no data available, this does leave the host
		// exposed to an attack by a wealthy renter whereby the renter causes
		// the host to lockup in-advance a bunch of funds that the renter then
		// never uses, meaning the host will not have collateral for other
		// clients.
		//
		// To mitigate the effects of this attack, the host has a collateral
		// fraction and a max collateral. CollateralFraction is a number that
		// gets divided by 1e6 and then represents the ratio of funds that the
		// host is willing to put into the contract relative to the number of
		// funds that the renter put into the contract. For example, if
		// 'CollateralFraction' is set to 1e6 and the renter adds 1 siacoin of
		// funding to the file contract, the host will also add 1 siacoin of
		// funding to the contract. if 'CollateralFraction' is set to 2e6, the
		// host would add 2 siacoins of funding to the contract.
		//
		// MaxCollateral indicates the maximum number of coins that a host is
		// willing to put into a file contract.
		Collateral            types.Currency `json:"collateral"`
		MaxCollateralFraction types.Currency `json:"maxcollateralfraction"`
		MaxCollateral         types.Currency `json:"maxcollateral"`

		// ContractPrice is the number of coins that the renter needs to pay to
		// the host just to open a file contract with them. Generally, the
		// price is only to cover the siacoin fees that the host will suffer
		// when submitting the file contract revision and storage proof to the
		// blockchain.
		//
		// The storage price is the cost per-byte-per-block in hastings of
		// storing data on the host.
		//
		// 'Download' bandwidth price is the cost per byte of downloading data
		// from the host.
		//
		// 'Upload' bandwidth price is the cost per byte of uploading data to
		// the host.
		ContractPrice          types.Currency `json:"contractprice"`
		DownloadBandwidthPrice types.Currency `json:"downloadbandwidthprice"`
		StoragePrice           types.Currency `json:"storageprice"`
		UploadBandwidthPrice   types.Currency `json:"uploadbandwidthprice"`

		// Because the host has a public key, and settings are signed, and
		// because settings may be MITM'd, settings need a revision number so
		// that a renter can compare multiple sets of settings and determine
		// which is the most recent.
		RevisionNumber uint64 `json:"revisionnumber"`
		Version        string `json:"version"`
	}

	// A RevisionAction is a description of an edit to be performed on a file
	// contract. Three types are allowed, 'ActionDelecte', 'ActionInsert', and
	// 'ActionModify'. ActionDelete just takes a sector index, indicating which
	// sector is going to be deleted. ActionInsert takes a sector index, and a
	// full sector of data, indicating that a sector at the index should be
	// inserted with the provided data. 'Modify' revises the sector at the
	// given index, rewriting it with the provided data starting from the
	// 'offset' within the sector.
	//
	// Modify could be simulated with an insert and a delete, however an insert
	// requires a full sector to be uploaded, and a modify can be just a few
	// kb, which can be significantly faster.
	RevisionAction struct {
		Type        types.Specifier
		SectorIndex uint64
		Offset      uint64
		Data        []byte
	}
)

// WriteNegotiationAcceptance writes the 'accept' response to w (usually a
// net.Conn).
func WriteNegotiationAcceptance(w io.Writer) error {
	return encoding.WriteObject(w, AcceptResponse)
}

// WriteNegotiationRejection will write a rejection response to w (usually a
// net.Conn) and return the input error. If the write fails, the write error
// is joined with the input error.
func WriteNegotiationRejection(w io.Writer, err error) error {
	writeErr := encoding.WriteObject(w, err.Error())
	if writeErr != nil {
		return build.JoinErrors([]error{err, writeErr}, "; ")
	}
	return err
}

// ReadNegotiationAcceptance reads an accept/reject response from r (usually a
// net.Conn). If the response is not acceptance, ReadNegotiationAcceptance
// returns the response as an error.
//
// Note that since errors returned by ReadNegotiationAcceptance are newly
// allocated, they cannot be compared to other errors in the traditional
// fashion.
func ReadNegotiationAcceptance(r io.Reader) error {
	var resp string
	err := encoding.ReadObject(r, &resp, MaxErrorSize)
	if err != nil {
		return err
	} else if resp != AcceptResponse {
		return errors.New(resp)
	}
	return nil
}

// CreateAnnouncement will take a host announcement and encode it, returning
// the exact []byte that should be added to the arbitrary data of a
// transaction.
func CreateAnnouncement(addr NetAddress, pk types.SiaPublicKey, sk crypto.SecretKey) (signedAnnouncement []byte, err error) {
	// Create the HostAnnouncement and marshal it.
	annBytes := encoding.Marshal(HostAnnouncement{
		Specifier:  PrefixHostAnnouncement,
		NetAddress: addr,
		PublicKey:  pk,
	})

	// Create a signature for the announcement.
	annHash := crypto.HashBytes(annBytes)
	sig, err := crypto.SignHash(annHash, sk)
	if err != nil {
		return nil, err
	}
	// Return the signed announcement.
	return append(annBytes, sig[:]...), nil
}

// DecodeAnnouncement decodes announcement bytes into a host announcement,
// verifying the prefix and the signature.
func DecodeAnnouncement(fullAnnouncement []byte) (na NetAddress, spk types.SiaPublicKey, err error) {
	// Read the first part of the announcement to get the intended host
	// announcement.
	var ha HostAnnouncement
	dec := encoding.NewDecoder(bytes.NewReader(fullAnnouncement))
	err = dec.Decode(&ha)
	if err != nil {
		return "", types.SiaPublicKey{}, err
	}

	// Check that the announcement was registered as a host announcement.
	if ha.Specifier != PrefixHostAnnouncement {
		return "", types.SiaPublicKey{}, ErrAnnNotAnnouncement
	}
	// Check that the public key is a recognized type of public key.
	if ha.PublicKey.Algorithm != types.SignatureEd25519 {
		return "", types.SiaPublicKey{}, ErrAnnUnrecognizedSignature
	}

	// Read the signature out of the reader.
	var sig crypto.Signature
	err = dec.Decode(&sig)
	if err != nil {
		return "", types.SiaPublicKey{}, err
	}
	// Verify the signature.
	var pk crypto.PublicKey
	copy(pk[:], ha.PublicKey.Key)
	annHash := crypto.HashObject(ha)
	err = crypto.VerifyHash(annHash, pk, sig)
	if err != nil {
		return "", types.SiaPublicKey{}, err
	}
	return ha.NetAddress, ha.PublicKey, nil
}
