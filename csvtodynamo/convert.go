package csvtodynamo

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"

	"github.com/aws/aws-sdk-go/service/dynamodb"
)

// Converter converts CSV to DynamoDB records.
type Converter struct {
	r                    *csv.Reader
	conf                 *Configuration
	columnNames          []string
	columnNamesToInclude map[string]bool
}

type keyConverter func(s string) *dynamodb.AttributeValue

// NewConfiguration creates the Configuration for the Converter.
func NewConfiguration() *Configuration {
	return &Configuration{
		KeyToConverter: map[string]keyConverter{},
	}
}

// Configuration for the Converter.
type Configuration struct {
	KeyToConverter map[string]keyConverter
	Columns        []string
	KeyColumns     []string
}

// AddStringKeys add string keys to the configuration.
func (conf *Configuration) AddStringKeys(s ...string) *Configuration {
	for _, k := range s {
		conf.KeyToConverter[k] = stringValue
	}
	return conf
}

// AddNumberKeys adds numeric keys to the configuration.
func (conf *Configuration) AddNumberKeys(s ...string) *Configuration {
	for _, k := range s {
		conf.KeyToConverter[k] = numberValue
	}
	return conf
}

// AddBoolKeys adds boolean keys to the configuration.
func (conf *Configuration) AddBoolKeys(s ...string) *Configuration {
	for _, k := range s {
		conf.KeyToConverter[k] = boolValue
	}
	return conf
}

func (conf *Configuration) AddMapKeys(s ...string) *Configuration {
	for _, k := range s {
		conf.KeyToConverter[k] = mapValue
	}
	return conf
}

func (conf *Configuration) AddBinKeys(s ...string) *Configuration {
	for _, k := range s {
		conf.KeyToConverter[k] = binValue
	}
	return conf
}

func (conf *Configuration) AddKeyColumns(s ...string) *Configuration {
	for _, k := range s {
		conf.KeyColumns = append(conf.KeyColumns, k)
	}
	return conf
}

func (c *Converter) init() error {
	if len(c.conf.KeyColumns) > 0 {
		c.columnNamesToInclude = make(map[string]bool)
		for _, k := range c.conf.KeyColumns {
			c.columnNamesToInclude[k] = true
		}
	}
	if len(c.conf.Columns) > 0 {
		c.columnNames = c.conf.Columns
		return nil
	}
	record, err := c.r.Read()
	if err != nil {
		return err
	}
	if c.columnNames == nil {
		c.columnNames = record
	}
	return nil
}

// ReadBatch reads 25 items from the CSV.
// Only strings, numbers and boolean values are supported in CSV.
func (c *Converter) ReadBatch() (items []map[string]*dynamodb.AttributeValue, read int, err error) {
	batchSize := 25
	items = make([]map[string]*dynamodb.AttributeValue, batchSize)
	for read = 0; read < batchSize; read++ {
		items[read], err = c.Read()
		if err != nil {
			break
		}
	}
	return items[:read], read, err
}

// Read a single item from the CSV.
func (c *Converter) Read() (items map[string]*dynamodb.AttributeValue, err error) {
	record, err := c.r.Read()
	if err != nil {
		return
	}
	items = make(map[string]*dynamodb.AttributeValue, len(record))
	for i, column := range c.columnNames {
		if len(c.columnNamesToInclude) > 0 && !c.columnNamesToInclude[column] {
			continue
		}
		if len(record[i]) != 0 {
			items[column] = c.dynamoValue(column, record[i])
		}
	}
	return items, err
}

// NewConverter creates a new CSV to DynamoDB converter.
func NewConverter(r *csv.Reader, conf *Configuration) (*Converter, error) {
	if conf == nil {
		conf = NewConfiguration()
	}
	c := &Converter{
		r:    r,
		conf: conf,
	}
	err := c.init()
	return c, err
}

func (c *Converter) dynamoValue(key, value string) *dynamodb.AttributeValue {
	if f, ok := c.conf.KeyToConverter[key]; ok {
		return f(value)
	}
	return stringValue(value)
}

func stringValue(s string) *dynamodb.AttributeValue {
	return (&dynamodb.AttributeValue{}).SetS(s)
}

func numberValue(s string) *dynamodb.AttributeValue {
	return (&dynamodb.AttributeValue{}).SetN(s)
}

func boolValue(s string) *dynamodb.AttributeValue {
	if v, ok := boolValues[s]; ok {
		return v
	}
	return falseValue
}

func mapValue(s string) *dynamodb.AttributeValue {
	var av map[string]*dynamodb.AttributeValue
	json.Unmarshal([]byte(s), &av)
	return (&dynamodb.AttributeValue{}).SetM(av)
}

func binValue(s string) *dynamodb.AttributeValue {
	b, _ := base64.StdEncoding.DecodeString(s)
	return (&dynamodb.AttributeValue{}).SetB(b)
}

var trueValue = (&dynamodb.AttributeValue{}).SetBOOL(true)
var falseValue = (&dynamodb.AttributeValue{}).SetBOOL(false)

var boolValues = map[string]*dynamodb.AttributeValue{
	"false": falseValue,
	"FALSE": falseValue,
	"true":  trueValue,
	"TRUE":  trueValue,
}
