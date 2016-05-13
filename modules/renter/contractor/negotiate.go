package contractor

import (
	"errors"
	"net"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// verifySettings reads a signed HostSettings object from conn, validates the
// signature, and checks for discrepancies between the known settings and the
// received settings. If there is a discrepancy, the hostDB is notified. The
// received settings are returned.
func verifySettings(conn net.Conn, host modules.HostDBEntry, hdb hostDB) (modules.HostDBEntry, error) {
	// convert host key (types.SiaPublicKey) to a crypto.PublicKey
	if host.PublicKey.Algorithm != types.SignatureEd25519 || len(host.PublicKey.Key) != crypto.PublicKeySize {
		build.Critical("hostdb did not filter out host with wrong signature algorithm:", host.PublicKey.Algorithm)
		return modules.HostDBEntry{}, errors.New("host used unsupported signature algorithm")
	}
	var pk crypto.PublicKey
	copy(pk[:], host.PublicKey.Key)

	// read signed host settings
	var recvSettings modules.HostExternalSettings
	if err := crypto.ReadSignedObject(conn, &recvSettings, modules.NegotiateMaxHostExternalSettingsLen, pk); err != nil {
		return modules.HostDBEntry{}, errors.New("couldn't read host's settings: " + err.Error())
	}
	// TODO: check recvSettings against host.HostExternalSettings. If there is
	// a discrepancy, write the error to conn and update the hostdb
	if recvSettings.NetAddress != host.NetAddress {
		// for now, just overwrite the NetAddress, since we know that
		// host.NetAddress works (it was the one we dialed to get conn)
		recvSettings.NetAddress = host.NetAddress
	}
	host.HostExternalSettings = recvSettings
	return host, nil
}

// startRevision is run at the beginning of each revision iteration. It reads
// the host's settings confirms that the values are acceptable, and writes an acceptance.
func startRevision(conn net.Conn, host modules.HostDBEntry, hdb hostDB) error {
	// verify the host's settings and confirm its identity
	// TODO: return new host, so we can calculate price accurately
	recvSettings, err := verifySettings(conn, host, hdb)
	if err != nil {
		return err
	} else if !recvSettings.AcceptingContracts {
		// no need to reject; host will already have disconnected at this point
		return errors.New("host is not accepting contracts")
	}
	return modules.WriteNegotiationAcceptance(conn)
}

// startDownload is run at the beginning of each download iteration. It reads
// the host's settings confirms that the values are acceptable, and writes an acceptance.
func startDownload(conn net.Conn, host modules.HostDBEntry, hdb hostDB) error {
	// verify the host's settings and confirm its identity
	// TODO: return new host, so we can calculate price accurately
	_, err := verifySettings(conn, host, hdb)
	if err != nil {
		// TODO: doesn't make sense to reject here if the err is an I/O error.
		return modules.WriteNegotiationRejection(conn, err)
	}
	return modules.WriteNegotiationAcceptance(conn)
}

// verifyRecentRevision confirms that the host and contractor agree upon the current
// state of the contract being revisde.
func verifyRecentRevision(conn net.Conn, contract Contract) error {
	// send contract ID
	if err := encoding.WriteObject(conn, contract.ID); err != nil {
		return errors.New("couldn't send contract ID: " + err.Error())
	}
	// read challenge
	var challenge crypto.Hash
	if err := encoding.ReadObject(conn, &challenge, 32); err != nil {
		return errors.New("couldn't read challenge: " + err.Error())
	}
	// sign and return
	sig, err := crypto.SignHash(challenge, contract.SecretKey)
	if err != nil {
		return err
	} else if err := encoding.WriteObject(conn, sig); err != nil {
		return errors.New("couldn't send challenge response: " + err.Error())
	}
	// read acceptance
	if err := modules.ReadNegotiationAcceptance(conn); err != nil {
		return errors.New("host did not accept revision request: " + err.Error())
	}
	// read last revision and signatures
	var lastRevision types.FileContractRevision
	var hostSignatures []types.TransactionSignature
	if err := encoding.ReadObject(conn, &lastRevision, 2048); err != nil {
		return errors.New("couldn't read last revision: " + err.Error())
	}
	if err := encoding.ReadObject(conn, &hostSignatures, 2048); err != nil {
		return errors.New("couldn't read host signatures: " + err.Error())
	}
	// verify the revision and signatures
	// NOTE: we can fake the blockheight here because it doesn't affect
	// verification; it just needs to be above the fork height and below the
	// contract expiration (which was checked earlier).
	return modules.VerifyFileContractRevisionTransactionSignatures(lastRevision, hostSignatures, contract.FileContract.WindowStart-1)
}
