package main

import (
	"fmt"

	"aidanwoods.dev/go-paseto"
)

func main() {
	fmt.Println("=== PASETO v4 Asymmetric Key Generator (Ed25519) ===")
	
	// Generate a new PASETO v4 asymmetric secret key (Ed25519)
	secretKey := paseto.NewV4AsymmetricSecretKey()
	
	// Extract the public key
	publicKey := secretKey.Public()
	
	fmt.Println("\n-- SECRET KEY --")
	fmt.Println("Simpan ini di environment variable: PASETO_PRIVATE_KEY")
	fmt.Println(secretKey.ExportHex())
	fmt.Println("\n-- PUBLIC KEY --")
	fmt.Println("Simpan ini di environment variable: PASETO_PUBLIC_KEY")
	fmt.Println(publicKey.ExportHex())
	fmt.Println("\nPERHATIAN: Rahasiakan Secret Key Anda. Public Key dapat dibagikan.")
}
