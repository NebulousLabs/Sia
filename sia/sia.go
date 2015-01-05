// The sia package is made up of multiple components, each implemented using an
// interface. The list:
//		Host
//		HostDB
//		Miner
//		Renter
//		Wallet
//
// Each of these interfaces contains an Init() function. Calling Init() will
// clear the interface and prepare it to get information from the beginning of
// consensus.
//
// Each interface also contains an Update() function, which allows the core to
// dump new information to the interface. Init() is used in conjunction with
// Update(), and informs the underlying struct that it's about to get a
// completely fresh set of information. Standard procedure is to call Init() a
// single time, and then to repeatedly call Update().
//
// Each interface also contains an Info() function, which returns a []byte
// containing information internal to the interface. What is returned is
// completely unregulated by the Core, and is for a frontend to parse.
package sia
