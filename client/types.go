package client

import (
	"math/big"
	"time"

	"github.com/bnb-chain/tss-lib/v2/crypto/paillier"
)

// PreParamsData contains all pre-computed parameters for ECDSA DKG
type PreParamsData struct {
	PaillierKey *paillier.PrivateKey
	NTildei     *big.Int
	H1i         *big.Int
	H2i         *big.Int
	Alpha       *big.Int
	Beta        *big.Int
	P           *big.Int // safe prime for NTildei
	Q           *big.Int // safe prime for NTildei
	GeneratedAt time.Time
}