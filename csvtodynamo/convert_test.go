package csvtodynamo

import (
	"encoding/base64"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/google/go-cmp/cmp"
)

func TestConverter(t *testing.T) {
	const binAsBase64 = "F9vBa7O+Ee6/7gJCrGMAFA=="
	bin, _ := base64.StdEncoding.DecodeString(binAsBase64)
	var tests = []struct {
		name          string
		input         string
		config        *Configuration
		expected      []map[string]*dynamodb.AttributeValue
		expectedError error
	}{
		{
			name: "wrong number of fields",
			input: strings.Join([]string{
				"a,b,c",
				"1,2,3,4",
			}, "\n"),
			expectedError: csv.ErrFieldCount,
		},
		{
			name: "numbers are not identified by default",
			input: strings.Join([]string{
				"a,b,c",
				"1,2.12,-3",
			}, "\n"),
			expected: []map[string]*dynamodb.AttributeValue{
				{
					"a": &dynamodb.AttributeValue{S: aws.String("1")},
					"b": &dynamodb.AttributeValue{S: aws.String("2.12")},
					"c": &dynamodb.AttributeValue{S: aws.String("-3")},
				},
			},
		},
		{
			name: "numbers can be configured",
			input: strings.Join([]string{
				"a,b,c,d",
				"1,2.12,2.12,-3",
			}, "\n"),
			config: NewConfiguration().AddNumberKeys("a", "c", "d"),
			expected: []map[string]*dynamodb.AttributeValue{
				{
					"a": &dynamodb.AttributeValue{N: aws.String("1")},
					"b": &dynamodb.AttributeValue{S: aws.String("2.12")},
					"c": &dynamodb.AttributeValue{N: aws.String("2.12")},
					"d": &dynamodb.AttributeValue{N: aws.String("-3")},
				},
			},
		},
		{
			name: "bools can be identified",
			input: strings.Join([]string{
				"a,b,c,d",
				"TRUE,FALSE,true,false",
			}, "\n"),
			config: NewConfiguration().AddBoolKeys("a", "b", "c", "d"),
			expected: []map[string]*dynamodb.AttributeValue{
				{
					"a": &dynamodb.AttributeValue{BOOL: aws.Bool(true)},
					"b": &dynamodb.AttributeValue{BOOL: aws.Bool(false)},
					"c": &dynamodb.AttributeValue{BOOL: aws.Bool(true)},
					"d": &dynamodb.AttributeValue{BOOL: aws.Bool(false)},
				},
			},
		},
		{
			name: "maps can be identified",
			input: strings.Join([]string{
				"one,three,four",
				`"{""one"":{""N"":""1""},""two"":{""S"":""2""}}","{""three"":{""N"":""3""}}","{""four"":{""M"":{""five"":{""N"":""5""}}}}"`,
			}, "\n"),
			config: NewConfiguration().AddMapKeys("one", "four"),
			expected: []map[string]*dynamodb.AttributeValue{
				{
					"one": &dynamodb.AttributeValue{M: map[string]*dynamodb.AttributeValue{
						"one": {N: aws.String("1")},
						"two": {S: aws.String("2")},
					}},
					"three": &dynamodb.AttributeValue{S: aws.String(`{"three":{"N":"3"}}`)},
					"four": &dynamodb.AttributeValue{M: map[string]*dynamodb.AttributeValue{
						"four": {M: map[string]*dynamodb.AttributeValue{
							"five": {N: aws.String("5")},
						}},
					}},
				},
			},
		},
		{
			name: "binary can be identified",
			input: strings.Join([]string{
				"one,two",
				"1,\"F9vBa7O+Ee6/7gJCrGMAFA==\"",
			}, "\n"),
			config: NewConfiguration().AddBinKeys("two"),
			expected: []map[string]*dynamodb.AttributeValue{
				{
					"one": &dynamodb.AttributeValue{S: aws.String("1")},
					"two": &dynamodb.AttributeValue{B: bin},
				},
			},
		},
		{
			name: "strings are handled",
			input: strings.Join([]string{
				"a,b,c",
				`the,"red, wine",cork`,
			}, "\n"),
			expected: []map[string]*dynamodb.AttributeValue{
				{
					"a": &dynamodb.AttributeValue{S: aws.String("the")},
					"b": &dynamodb.AttributeValue{S: aws.String("red, wine")},
					"c": &dynamodb.AttributeValue{S: aws.String("cork")},
				},
			},
		},
		{
			name: "various types are handled",
			input: strings.Join([]string{
				"a,b,c",
				`1.1.1,false,123`,
			}, "\n"),
			config: NewConfiguration().AddBoolKeys("b").AddNumberKeys("c"),
			expected: []map[string]*dynamodb.AttributeValue{
				{
					"a": &dynamodb.AttributeValue{S: aws.String("1.1.1")},
					"b": &dynamodb.AttributeValue{BOOL: aws.Bool(false)},
					"c": &dynamodb.AttributeValue{N: aws.String("123")},
				},
			},
		},
		{
			name: "empty values are not included",
			input: strings.Join([]string{
				"a,b,c",
				`the,,cork`,
			}, "\n"),
			expected: []map[string]*dynamodb.AttributeValue{
				{
					"a": &dynamodb.AttributeValue{S: aws.String("the")},
					"c": &dynamodb.AttributeValue{S: aws.String("cork")},
				},
			},
		},
		{
			name: "when specifying key columns, other columns are ignored",
			input: strings.Join([]string{
				"a,b,c",
				`the,"red, wine",cork`,
			}, "\n"),
			config: NewConfiguration().AddKeyColumns("b", "c"),
			expected: []map[string]*dynamodb.AttributeValue{
				{
					"b": &dynamodb.AttributeValue{S: aws.String("red, wine")},
					"c": &dynamodb.AttributeValue{S: aws.String("cork")},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			r := csv.NewReader(strings.NewReader(tt.input))
			c, err := NewConverter(r, tt.config)
			if err != nil {
				if diff := cmp.Diff(tt.expectedError, err); diff != "" {
					t.Error("unexpected error")
					t.Fatal(diff)
				}
			}
			actual, read, err := c.ReadBatch()
			if err != io.EOF && tt.expectedError == nil {
				t.Fatalf("unexpected error: %v", err)
				return
			}
			if tt.expectedError != nil {
				if !errors.Is(err, tt.expectedError) {
					t.Fatalf("incorrect error, expected %v, got %v", tt.expectedError, err)
				}
				return
			}
			if diff := cmp.Diff(tt.expected, actual[:read]); diff != "" {
				t.Error("unexpected result")
				t.Error(diff)
			}
			if len(tt.expected) != read {
				t.Errorf("expected %d reads, read %d", len(tt.expected), read)
			}
		})
	}

}
