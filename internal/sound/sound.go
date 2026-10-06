// Package sound проигрывает короткий сигнал при переключении раскладки.
package sound

import (
	"bytes"
	"encoding/binary"
	"math"
)

// click — WAV (16 бит, моно): двухнотный сигнал (A5 → E6) длиной ~350 мс.
// Bluetooth-наушники «просыпаются» с задержкой и съедают начало звука,
// поэтому вторая нота звучит позже и долетает даже в этом случае.
var click = func() []byte {
	const (
		rate = 44100
		dur  = 0.35
	)
	notes := []struct{ start, freq float64 }{
		{0, 880},
		{0.12, 1318.5},
	}
	n := int(rate * dur)
	samples := make([]int16, n)
	for i := range n {
		t := float64(i) / rate
		var v float64
		for _, note := range notes {
			if dt := t - note.start; dt >= 0 {
				attack := min(dt/0.005, 1) // плавное начало без щелчка
				v += attack * math.Exp(-dt*14) * math.Sin(2*math.Pi*note.freq*dt)
			}
		}
		samples[i] = int16(0.2 * math.MaxInt16 * max(-1, min(v, 1)))
	}

	var buf bytes.Buffer
	w := func(v any) { _ = binary.Write(&buf, binary.LittleEndian, v) } // запись в bytes.Buffer не возвращает ошибок
	buf.WriteString("RIFF")
	w(uint32(36 + 2*n))
	buf.WriteString("WAVEfmt ")
	w(uint32(16))       // размер блока fmt
	w(uint16(1))        // PCM
	w(uint16(1))        // каналов
	w(uint32(rate))     // частота
	w(uint32(rate * 2)) // байт в секунду
	w(uint16(2))        // байт на сэмпл
	w(uint16(16))       // бит на сэмпл
	buf.WriteString("data")
	w(uint32(2 * n))
	w(samples)
	return buf.Bytes()
}()
