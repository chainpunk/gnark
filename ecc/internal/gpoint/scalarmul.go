package gpoint

const scalarMul = `
// ScalarMul multiplies a by scalar
// algorithm: a special case of Pippenger described by Bootle:
// https://jbootle.github.io/Misc/pippenger.pdf
func (p *{{.Name}}Jac) ScalarMul(curve *Curve, a *{{.Name}}Jac, scalar fr.Element) *{{.Name}}Jac {
	// see MultiExp and pippenger documentation for more details about these constants / variables
	const s = 4
	const b = s
	const TSize = (1 << b) - 1
	var T [TSize]{{.Name}}Jac
	computeT := func(T []{{.Name}}Jac, t0 *{{.Name}}Jac) {
		T[0].Set(t0)
		for j := 1; j < (1<<b)-1; j = j + 2 {
			T[j].Set(&T[j/2]).Double()
			T[j+1].Set(&T[(j+1)/2]).Add(curve, &T[j/2])
		}
	}
	return p.pippenger(curve, []{{.Name}}Jac{*a}, []fr.Element{scalar}, s, b, T[:], computeT)
}
// ScalarMulByGen multiplies curve.{{toLower .Name}}Gen by scalar
// algorithm: a special case of Pippenger described by Bootle:
// https://jbootle.github.io/Misc/pippenger.pdf
func (p *{{.Name}}Jac) ScalarMulByGen(curve *Curve, scalar fr.Element) *{{.Name}}Jac {
	computeT := func(T []{{.Name}}Jac, t0 *{{.Name}}Jac) {}
	return p.pippenger(curve, []{{.Name}}Jac{curve.{{toLower .Name}}Gen}, []fr.Element{scalar}, sGen, bGen, curve.tGen{{.Name}}[:], computeT)
}
`

const multiExp = `
func (p *{{.Name}}Jac) MultiExp(curve *Curve, points []{{ .Name}}Affine, scalars []fr.Element) chan {{.Name}}Jac  {
	debug.Assert(len(scalars) == len(points))
	chRes := make(chan {{.Name}}Jac, 1)
	// call windowed multi exp if input not large enough
	// we may want to force the API user to call the proper method in the first place
	const minPoints = 50 				// under 50 points, the windowed multi exp performs better
	if len(scalars) <= minPoints {
		_points := make([]{{ .Name}}Jac, len(points))
		for i := 0; i < len(points); i++ {
			points[i].ToJacobian(&_points[i])
		}
		go func() {
			p.WindowedMultiExp(curve, _points, scalars)
			chRes <- *p
		}()
		return chRes
		
	}
	// compute nbCalls and nbPointsPerBucket as a function of available CPUs
	const chunkSize = 64
	const totalSize = chunkSize * fr.ElementLimbs
	var nbBits, nbCalls uint64
	nbPoints := len(scalars)
	nbPointsPerBucket := 20		// empirical parameter to chose nbBits
	// set nbBbits and nbCalls
	nbBits = 0
	for len(scalars)/(1<<nbBits) >= nbPointsPerBucket {
		nbBits++
	}
	nbCalls = totalSize / nbBits
	if totalSize%nbBits > 0 {
		nbCalls++
	}
	const useAllCpus = false 
	// if we need to use all CPUs
	if useAllCpus {
		nbCpus := uint64(runtime.NumCPU())
		// goal here is to have at least as many calls as number of go routine we're allowed to spawn
		for nbCalls < nbCpus && nbPointsPerBucket < nbPoints {
			nbBits = 0
			for len(scalars)/(1<<nbBits) >= nbPointsPerBucket {
				nbBits++
			}
			nbCalls = totalSize / nbBits
			if totalSize%nbBits > 0 {
				nbCalls++
			}
			nbPointsPerBucket *= 2
		}
	}
	
	// result (1 per go routine)
	tmpRes := make([]{{ .Name}}Jac, nbCalls)
	work := func(iStart, iEnd int) {
		chunks := make([]uint64, nbBits)
		offsets := make([]uint64, nbBits)
		for i := uint64(iStart); i < uint64(iEnd); i++ {
			start := i * nbBits
			debug.Assert(start != totalSize)
			var counter uint64
			for j := start;counter < nbBits  && (j < totalSize);j++ {
				chunks[counter] = j/chunkSize
				offsets[counter] = j%chunkSize
				counter++
			}
			c := 1 << counter
			buckets := make([]{{ .Name}}Jac, c-1)
			for j := 0; j < c-1; j++ {
				buckets[j].X.SetOne()
				buckets[j].Y.SetOne()
			}
			var l uint64
			for j := 0; j < nbPoints; j++ {
				var index uint64
				for k := uint64(0); k < counter; k++ {
					l = scalars[j][chunks[k]] >> offsets[k]
					l &= 1
					l <<= k
					index += l
				}
				if index != 0 {
					buckets[index-1].AddMixed(&points[j])
				} 
			}
			sum := curve.{{toLower .Name}}Infinity
			for j := len(buckets) - 1; j >= 0; j-- {
				sum.Add(curve, &buckets[j])
				tmpRes[i].Add(curve, &sum)
			}
		}
	}
	chDone := pool.ExecuteAsync(0, len(tmpRes), work, false)
	 go func() {
		<-chDone // that's making a "go routine" in the pool block, uncool
		p.Set(&curve.{{toLower .Name}}Infinity)
		for i := len(tmpRes) - 1; i >= 0; i-- {
			for j := uint64(0); j < nbBits; j++ {
				p.Double()
			}
			p.Add(curve, &tmpRes[i])
		}
		chRes <- *p
	}()
	return chRes
}
// MultiExp set p = scalars[0]*points[0] + ... + scalars[n]*points[n]
// see: https://eprint.iacr.org/2012/549.pdf
// if maxGoRoutine is not provided, uses all available CPUs
func (p *{{.Name}}Jac) MultiExpNew(curve *Curve, points []{{ .Name}}Affine, scalars []fr.Element) chan {{.Name}}Jac  {
	debug.Assert(len(scalars) == len(points))
	chRes := make(chan {{.Name}}Jac, 1)
	// call windowed multi exp if input not large enough
	// we may want to force the API user to call the proper method in the first place
	const minPoints = 50 				// under 50 points, the windowed multi exp performs better
	if len(scalars) <= minPoints {
		_points := make([]{{ .Name}}Jac, len(points))
		for i := 0; i < len(points); i++ {
			points[i].ToJacobian(&_points[i])
		}
		go func() {
			p.WindowedMultiExp(curve, _points, scalars)
			chRes <- *p
		}()
		return chRes
	}
	// if we have m points and n cpus, m/n points per cpu
	nbCpus := runtime.NumCPU()
	// nb points processed by one cpu
	pointsPerCPU := len(scalars) / nbCpus
	// each cpu has its own bucket
	sharedBuckets := make([][32][255]{{.Name}}Jac, nbCpus)
	// bucket to gather cpus work
	var commonBucket [32][255]{{.Name}}Jac
	var emptyBucket [255]{{.Name}}Jac
	var almostThere [32]{{.Name}}Jac
	for i := 0; i < 255; i++ {
		emptyBucket[i].Set(&curve.{{toLower .Name}}Infinity)
	}
	
	const mask = 255
	// id: cpu id, start, end: point nb start to point nb end, chunk: i-th digit (in corresponding basis)
	worker := func(id, start, end int) {
		// ...[chunk*8..(chunk+1)*8-1]... -th bits
		for chunk := 0; chunk < 32; chunk++ {
			limb := chunk / 8
			offset := (chunk % 8) * 8
			sharedBuckets[id][chunk] = emptyBucket
			var index uint64
			for i := start; i < end; i++ {
				index = (scalars[i][limb] >> offset)
				index &= mask
				if index != 0 {
					sharedBuckets[id][chunk][index-1].AddMixed(&points[i])
				}
			}
		}
	}
	var wg sync.WaitGroup
	
	// each cpu works on a small part of the bucket
	for j := 0; j < nbCpus; j++ {
		var nextStart, nextEnd int
		nextStart = j * pointsPerCPU
		if j < nbCpus-1 {
			nextEnd = nextStart + pointsPerCPU
		} else {
			nextEnd = len(scalars)
		}
		_j := j
		wg.Add(1)
		pool.Push(func() {
			worker(_j, nextStart, nextEnd)
			wg.Done()
		}, false)
	}
	go func() {
		for i := 0; i < 32; i++ {
			// initialize the common bucket for the current chunk
			commonBucket[i] = emptyBucket
		}
		copy(almostThere[:], emptyBucket[:])
		wg.Wait()
		// ...[chunk*8..(chunk+1)*8-1]... -th bits
		for i := 0; i < 32; i++ {
			// fill the i-th chunk of the common bucket by gathering cpus work
			for j := 0; j < 255; j++ {
				for k := 0; k < nbCpus; k++ {
					commonBucket[i][j].Add(curve, &sharedBuckets[k][i][j])
				}
			}
		}
	
		// ...[chunk*8..(chunk+1)*8-1]... -th bits
		var acc {{.Name}}Jac
		for i := 0; i < 32; i++ {
			acc.Set(&curve.{{toLower .Name}}Infinity)
			for j := 254; j >= 0; j-- {
				acc.Add(curve, &commonBucket[i][j])
				almostThere[i].Add(curve, &acc)
			}
		}
	
		// double and add to compute p
		p.Set(&curve.{{toLower .Name}}Infinity)
		for i := 31; i >= 0; i-- {
			for j := 0; j < 8; j++ {
				p.Double()
			}
			p.Add(curve, &almostThere[i])
		}
		chRes <- *p
	}()
	
	return chRes
}
`

const windowedMultiExp = `
// WindowedMultiExp set p = scalars[0]*points[0] + ... + scalars[n]*points[n]
// assume: scalars in non-Montgomery form!
// assume: len(points)==len(scalars)>0, len(scalars[i]) equal for all i
// algorithm: a special case of Pippenger described by Bootle:
// https://jbootle.github.io/Misc/pippenger.pdf
// uses all availables runtime.NumCPU()
func (p *{{.Name}}Jac) WindowedMultiExp(curve *Curve, points []{{.Name}}Jac, scalars []fr.Element) *{{.Name}}Jac {
	var lock sync.Mutex
	pool.Execute(0, len(points), func(start, end int) {
		var t {{.Name}}Jac
		t.multiExp(curve, points[start:end], scalars[start:end])
		lock.Lock()
		p.Add(curve, &t)
		lock.Unlock()
	}, false)
	return p
}
// multiExp set p = scalars[0]*points[0] + ... + scalars[n]*points[n]
// assume: scalars in non-Montgomery form!
// assume: len(points)==len(scalars)>0, len(scalars[i]) equal for all i
// algorithm: a special case of Pippenger described by Bootle:
// https://jbootle.github.io/Misc/pippenger.pdf
func (p *{{.Name}}Jac) multiExp(curve *Curve, points []{{.Name}}Jac, scalars []fr.Element) *{{.Name}}Jac {
	const s = 4 // s from Bootle, we choose s divisible by scalar bit length
	const b = s // b from Bootle, we choose b equal to s
	// WARNING! This code breaks if you switch to b!=s
	// Because we chose b=s, each set S_i from Bootle is simply the set of points[i]^{2^j} for each j in [0:s]
	// This choice allows for simpler code
	// If you want to use b!=s then the S_i from Bootle are different
	const TSize = (1 << b) - 1 // TSize is size of T_i sets from Bootle, equal to 2^b - 1
	// Store only one set T_i at a time---don't store them all!
	var T [TSize]{{.Name}}Jac // a set T_i from Bootle, the set of g^j for j in [1:2^b] for some choice of g
	computeT := func(T []{{.Name}}Jac, t0 *{{.Name}}Jac) {
		T[0].Set(t0)
		for j := 1; j < (1<<b)-1; j = j + 2 {
			T[j].Set(&T[j/2]).Double()
			T[j+1].Set(&T[(j+1)/2]).Add(curve, &T[j/2])
		}
	}
	return p.pippenger(curve, points, scalars, s, b, T[:], computeT)
}
// algorithm: a special case of Pippenger described by Bootle:
// https://jbootle.github.io/Misc/pippenger.pdf
func (p *{{.Name}}Jac) pippenger(curve *Curve, points []{{.Name}}Jac, scalars []fr.Element, s, b uint64, T []{{.Name}}Jac, computeT func(T []{{.Name}}Jac, t0 *{{.Name}}Jac)) *{{.Name}}Jac {
	var t, selectorIndex, ks int
	var selectorMask, selectorShift, selector uint64
	
	t = fr.ElementLimbs * 64 / int(s) // t from Bootle, equal to (scalar bit length) / s
	selectorMask = (1 << b) - 1 // low b bits are 1
	morePoints := make([]{{.Name}}Jac, t)       // morePoints is the set of G'_k points from Bootle
	for k := 0; k < t; k++ {
		morePoints[k].Set(&curve.{{toLower .Name}}Infinity)
	}
	for i := 0; i < len(points); i++ {
		// compute the set T_i from Bootle: all possible combinations of elements from S_i from Bootle
		computeT(T, &points[i])
		// for each morePoints: find the right T element and add it
		for k := 0; k < t; k++ {
			ks = k * int(s)
			selectorIndex = ks / 64
			selectorShift = uint64(ks - (selectorIndex * 64))
			selector = (scalars[i][selectorIndex] & (selectorMask << selectorShift)) >> selectorShift
			if selector != 0 {
				morePoints[k].Add(curve, &T[selector-1])
			}
		}
	}
	// combine morePoints to get the final result
	p.Set(&morePoints[t-1])
	for k := t - 2; k >= 0; k-- {
		for j := uint64(0); j < s; j++ {
			p.Double()
		}
		p.Add(curve, &morePoints[k])
	}
	return p
}
`
