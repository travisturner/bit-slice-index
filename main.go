package main

import (
	"fmt"
	"math"
	"strconv"
)

type container []uint64
type bitslices []uint16

// Straight and simple C to Go translation from https://en.wikipedia.org/wiki/Hamming_weight
func popcount(x uint64) int {
	const (
		m1  = 0x5555555555555555 //binary: 0101...
		m2  = 0x3333333333333333 //binary: 00110011..
		m4  = 0x0f0f0f0f0f0f0f0f //binary:  4 zeros,  4 ones ...
		h01 = 0x0101010101010101 //the sum of 256 to the power of 0,1,2,3...
	)
	x -= (x >> 1) & m1             //put count of each 2 bits into those 2 bits
	x = (x & m2) + ((x >> 2) & m2) //put count of each 4 bits into those 4 bits
	x = (x + (x >> 4)) & m4        //put count of each 8 bits into those 8 bits
	return int((x * h01) >> 56)    //returns left 8 bits of x + (x<<8) + (x<<16) + (x<<24) + ...
}

func sum(bitmaps []uint16, filter uint16) uint64 {
	components := uint(16)

	Bnn := bitmaps[0] // not-null bitmap

	Bf := filter
	//Zi := Z0 // Zi is either all 0's or all 1's

	Bf = Bf & Bnn

	CNT := popcount(uint64(Bf))
	SUM := uint64(0)

	for i := uint(components); i > 0; i-- {
		//SUM += 2^(i-1) * (CNT - Count(Fᵢ AND Bᶠ))
		SUM += uint64(math.Pow(2, float64(i-1))) * uint64((CNT - popcount(uint64(bitmaps[i]&Bf))))
	}

	return SUM
}

func compare(bitmaps []uint16, comparisonType string, z uint16, filter uint16) uint16 {
	components := uint(16)

	Z0 := uint16(0x0000)
	Z1 := uint16(0xFFFF)

	Bnn := bitmaps[0] // not-null bitmap

	Bk := Z0 // keep
	Bf := filter
	Zi := Z0 // Zi is either all 0's or all 1's

	Bf = Bf & Bnn

	leadingDigits := true
	for i := uint(components); i > 0; i-- {
		Zs := (z << (components - i)) >> (components - 1)
		if Zs == 1 {
			Zi = Z1
		} else {
			Zi = Z0
		}

		// Ignore most significant bits that are all 0's.
		if (leadingDigits) && (bitmaps[i] == bitmaps[0]) && (Zi == Z0) {
			continue // we can skip the most significant zero valued bit positions
		}
		leadingDigits = false // no longer check for leading digits to skip

		if comparisonType == "GTE" || comparisonType == "GT" {
			//////////////  GTE, GT  ///////////////////
			// Add any bit positions (C) that we know we want to keep (Bᵏ) based on the fact
			// that Fᵢ for C is greater than Zᵢ. Only consider those C that we haven't
			// already determined to be excluded (Bᶠ).
			// Bᵏ = (Bᵏ OR (Fᵢ NOR Zᵢ₀₁)) AND Bᶠ
			Bk = (Bk | ^(bitmaps[i] | Zi)) & Bf

			// The only difference between > and >= is in the comparison on the final bit
			// position. Based on the comparison type, we may be able to return early.
			if comparisonType == "GT" && i == 1 {
				return Bk
			}

			// Remove any bit positions (C) that we have determined do not satisfy
			// the comparison with Z.
			// Note: an XOR in place of the NAND should logically give the same result
			// because we have already included the NOR (0-0) case in Bᵏ.
			//Bᶠ = (Bᶠ AND (Fᵢ NAND Zᵢ₀₁)) OR Bᵏ
			Bf = (Bf & ^(bitmaps[i] & Zi)) | Bk
		} else if comparisonType == "LTE" || comparisonType == "LT" {
			//////////////  LTE, LT  ///////////////////
			// Treat < and <= differently on the final bit position.
			if i == 1 {
				if comparisonType == "LT" {
					//Bᶠ = (Bᶠ AND (Fᵢ AND Zᵢ₀₁)) OR Bᵏ
					Bf = (Bf & (bitmaps[i] & Zi)) | Bk
				} else if comparisonType == "LTE" {
					Bf = (Bf & (bitmaps[i] | Zi)) | Bk
				}
			} else {
				// Add any bit positions (C) that we know we want to keep (Bᵏ) based on the fact
				// that Fᵢ for C is greater than Zᵢ. Only consider those C that we haven't
				// already determined to be excluded (Bᶠ).
				// Bᵏ = (Bᵏ OR (Fᵢ AND Zᵢ₀₁)) AND Bᶠ
				Bk = (Bk | (bitmaps[i] & Zi)) & Bf

				// Remove any bit positions (C) that we have determined do not satisfy
				// the comparison with Z.
				// Note: an XOR in place of the OR should logically give the same result
				// because we have already included the AND (1-1) case in Bᵏ.
				// Bᶠ = (Bᶠ AND (Fᵢ OR Zᵢ₀₁)) OR Bᵏ
				Bf = (Bf & (bitmaps[i] | Zi)) | Bk
			}
		} else if comparisonType == "EQ" {
			//Bᶠ = (Bᶠ AND (Fᵢ XOR Zᵢ₀₁))
			Bf = (Bf & (bitmaps[i] ^ Zi))
		}
	}
	return Bf
}

// Build the BSI representation of the bitmaps (this needs to be implemented in Pilosa on store of an integer)
func buildBSI(c container) bitslices {
	components := 16
	bsis := bitslices{} // the BSI, base-2, range-encoded representation
	bsis_str := []string{}

	for _, v := range c {
		fmt.Println("v:", v)
		bsi := v ^ 0xFFFF // flip the bits
		bsis = append(bsis, uint16(bsi))
	}

	for _, bsi := range bsis {
		x := strconv.FormatUint(uint64(bsi), 2)
		bsis_str = append(bsis_str, x)
		fmt.Println(x)
	}

	bb := make([]string, components+1)

	for _, bsi_str := range bsis_str {
		bb[0] += string("1") // not-null
		for i := 0; i < components; i++ {
			bb[i+1] = string(bsi_str[components-1-i]) + bb[i+1]
		}
	}

	b := make([]uint16, components+1)

	for i, bs := range bb {
		x, err := strconv.ParseUint(bs, 2, 16)
		if err != nil {
			fmt.Println("ERROR:", err)
			continue
		}
		b[i] = uint16(x)
	}
	return b
}

func main() {

	values := container{970, 860, 950, 41, 870}
	b := buildBSI(values)

	fmt.Println("b:")
	fmt.Println(b)
	fmt.Println("===========================")

	filter := uint16(0xFFFE)

	fmt.Println("LT 870:", compare(b, "LT", uint16(870), filter))
	fmt.Println("LTE 870:", compare(b, "LTE", uint16(870), filter))
	fmt.Println("GT 950:", compare(b, "GT", uint16(950), filter))
	fmt.Println("GT 950:", compare(b, "GTE", uint16(950), filter))
	fmt.Println("EQ 41:", compare(b, "EQ", uint16(41), filter))

	fmt.Println("SUM:", sum(b, filter))

}
