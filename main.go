package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"

	token "gb-sc-homework/contracts/IERC20"
)

type config struct {
	PrivateKey     string `env:"PRIVATE_KEY"`
	UserPrivateKey string `env:"USER_PRIVATE_KEY"`
	RpcNode        string `env:"BSCTESTNET_URL"`
}

type Account struct {
	PrivateKey ecdsa.PrivateKey
	PublicKey  ecdsa.PublicKey
	Address    common.Address
	Auth       *bind.TransactOpts
}

func ToDecimal(ivalue interface{}, decimals int) decimal.Decimal {
	value := new(big.Int)
	switch v := ivalue.(type) {
	case string:
		value.SetString(v, 10)
	case *big.Int:
		value = v
	}

	mul := decimal.NewFromFloat(float64(10)).Pow(decimal.NewFromFloat(float64(decimals)))
	num, _ := decimal.NewFromString(value.String())
	result := num.Div(mul)

	return result
}

// ToWei decimals to wei
func ToWei(iamount interface{}, decimals int) *big.Int {
	amount := decimal.NewFromFloat(0)
	switch v := iamount.(type) {
	case string:
		amount, _ = decimal.NewFromString(v)
	case float64:
		amount = decimal.NewFromFloat(v)
	case int64:
		amount = decimal.NewFromFloat(float64(v))
	case decimal.Decimal:
		amount = v
	case *decimal.Decimal:
		amount = *v
	}

	mul := decimal.NewFromFloat(float64(10)).Pow(decimal.NewFromFloat(float64(decimals)))
	result := amount.Mul(mul)

	wei := new(big.Int)
	wei.SetString(result.String(), 10)

	return wei
}

func getClient(rpcNode string) *ethclient.Client {

	client, err := ethclient.Dial(rpcNode)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("we have a connection")
	return client
}

func getAccount(privateKeyStr string, client *ethclient.Client) Account {

	privateKey, err := crypto.HexToECDSA(privateKeyStr)
	if err != nil {
		log.Fatal(err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("error casting public key to ECDSA")
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		log.Fatal(err)
	}
	auth.Nonce = nil
	auth.Value = big.NewInt(0)     // in wei
	auth.GasLimit = uint64(300000) // in units
	auth.GasPrice = gasPrice

	account := Account{
		PrivateKey: *privateKey,
		PublicKey:  *publicKeyECDSA,
		Address:    address,
		Auth:       auth,
	}
	return account
}

func checkTransactionReceipt(_txHash string, client ethclient.Client) int {
	txHash := common.HexToHash(_txHash)
	tx, err := client.TransactionReceipt(context.Background(), txHash)
	if err != nil {
		return (-1)
	}
	return (int(tx.Status))
}

func WaitForBlockCompletion(client ethclient.Client, hashToRead string) int {
	soc := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(context.Background(), soc)
	if err != nil {
		log.Fatal(err)
		return -1
	}

	for {
		select {
		case err := <-sub.Err():
			_ = err
			return -1
		case header := <-soc:
			fmt.Println(header.TxHash.Hex())
			transactionStatus := checkTransactionReceipt(hashToRead, client)
			if transactionStatus == 0 {
				//FAILURE
				sub.Unsubscribe()
				return 0
			} else if transactionStatus == 1 {
				//SUCCESS
				sub.Unsubscribe()
				return 1
			}
		}
	}
}

func main() {
	cfg := config{}
	godotenv.Load(".env")
	cfg.PrivateKey = os.Getenv("DEPLOYER_PRIVATE_KEY")
	cfg.UserPrivateKey = os.Getenv("USER_PRIVATE_KEY")
	cfg.RpcNode = os.Getenv("BSCTESTNET_URL")

	client := getClient(cfg.RpcNode)

	deployer := getAccount(cfg.PrivateKey, client)
	user := getAccount(cfg.UserPrivateKey, client)
	fmt.Println("Deployer address: ", deployer.Address.String())
	fmt.Println("User address: ", user.Address.String())

	tokenAddress := common.HexToAddress("0x8e374AbDFecEf1203BFC142FCA2E93819C98f2fC")
	tokenInstance, err := token.NewERC20token(tokenAddress, client)
	if err != nil {
		log.Fatal(err)
	}

	decimals, err := tokenInstance.Decimals(&bind.CallOpts{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("decimals: ", decimals) // "decimals: 18"

	//show balances
	deployerBalance, _ := tokenInstance.BalanceOf(&bind.CallOpts{}, deployer.Address)
	userBalance, _ := tokenInstance.BalanceOf(&bind.CallOpts{}, user.Address)

	fmt.Println("Deployer balance: ", ToDecimal(deployerBalance, int(decimals))) // "balance: 74605500.647409"
	fmt.Println("User balance: ", ToDecimal(userBalance, int(decimals)))         // "balance: 74605500.647409"

	//sending 100 tokens from deployer to user
	amount := ToWei(100.0, int(decimals))
	fmt.Println(amount)
	tx, err := tokenInstance.Transfer(deployer.Auth, user.Address, amount)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("tx sent: %s\n", tx.Hash().Hex())
	WaitForBlockCompletion(*client, tx.Hash().String())

	//show balances
	deployerBalance, _ = tokenInstance.BalanceOf(&bind.CallOpts{}, deployer.Address)
	userBalance, _ = tokenInstance.BalanceOf(&bind.CallOpts{}, user.Address)

	fmt.Println("Deployer balance: ", ToDecimal(deployerBalance, int(decimals))) // "balance: 74605500.647409"
	fmt.Println("User balance: ", ToDecimal(userBalance, int(decimals)))         // "balance: 74605500.647409"

	tx, err = tokenInstance.Approve(user.Auth, deployer.Address, amount)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("tx sent: %s\n", tx.Hash().Hex())
	WaitForBlockCompletion(*client, tx.Hash().String())

	amount = ToWei(10.0, int(decimals))
	tx, err = tokenInstance.TransferFrom(deployer.Auth, user.Address, deployer.Address, amount)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("tx sent: %s\n", tx.Hash().Hex())
	WaitForBlockCompletion(*client, tx.Hash().String())

	//show balances
	deployerBalance, _ = tokenInstance.BalanceOf(&bind.CallOpts{}, deployer.Address)
	userBalance, _ = tokenInstance.BalanceOf(&bind.CallOpts{}, user.Address)

	fmt.Println("Deployer balance: ", ToDecimal(deployerBalance, int(decimals))) // "balance: 74605500.647409"
	fmt.Println("User balance: ", ToDecimal(userBalance, int(decimals)))         // "balance: 74605500.647409"

}
