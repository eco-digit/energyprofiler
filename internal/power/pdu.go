package power

import (
	"fmt"
	"log"
	"net/url"
	"regexp"
	"time"

	"github.com/eco-digit/energyprofiler/internal/types"
	"github.com/simonvetter/modbus"
)

type PDUReader struct {
	verbose  bool
	register uint16
	factor   float64
	URL      url.URL
}

func NewPDUReader(modbusURL url.URL, register uint16, factor float64, verbose bool) *PDUReader {
	return &PDUReader{
		verbose:  verbose,
		register: register,
		factor:   factor,
		URL:      modbusURL,
	}
}

func (p *PDUReader) ReadPower(_ string) (float64, error) {
	return p.ReadSystemPower()
}

func (p *PDUReader) ReadSystemPower() (float64, error) {
	URL := p.URL.String()
	prefix := regexp.MustCompile(`\w+:\/\/`).FindString(URL)
	if prefix == "" {
		URL = fmt.Sprintf("tcp://%s", URL)
	}

	mc, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:     URL,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		return 0, err
	}

	if err := mc.Open(); err != nil {
		return 0, err
	}

	defer func() {
		if err := mc.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	rawValue, err := mc.ReadRegister(p.register, modbus.HOLDING_REGISTER)
	if err != nil {
		return 0, err
	}

	v := float64(rawValue) * p.factor
	if p.verbose {
		log.Printf("[pdu] System power (average): %.2f W", v)
	}

	return v, nil
}

// compile time compliance check
var _ types.PowerReader = (*PDUReader)(nil)
