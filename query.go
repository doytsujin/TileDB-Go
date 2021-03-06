package tiledb

/*
#cgo LDFLAGS: -ltiledb
#cgo linux LDFLAGS: -ldl
#include <tiledb/tiledb.h>
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"unsafe"
)

// Query construct and execute read/write queries on a tiledb Array
type Query struct {
	tiledbQuery          *C.tiledb_query_t
	array                *Array
	context              *Context
	uri                  string
	buffers              []interface{}
	bufferMutex          sync.Mutex
	resultBufferElements map[string][2]*uint64
}

// RangeLimits defines a query range
type RangeLimits struct {
	start interface{}
	end   interface{}
}

// MarshalJSON implements the Marshaler interface for RangeLimits
func (r RangeLimits) MarshalJSON() ([]byte, error) {
	rangeLimitMap := make(map[string]interface{}, 0)
	rangeLimitMap["end"] = r.end
	rangeLimitMap["start"] = r.start

	return json.Marshal(rangeLimitMap)
}

/*
NewQuery Creates a TileDB query object.

The query type (read or write) must be the same as the type used
to open the array object.

The storage manager also acquires a shared lock on the array.
This means multiple read and write queries to the same array can be made
concurrently (in TileDB, only consolidation requires an exclusive lock for
a short period of time).
*/
func NewQuery(ctx *Context, array *Array) (*Query, error) {
	if array == nil {
		return nil, fmt.Errorf("Error creating tiledb query: passed array is nil")
	}

	queryType, err := array.QueryType()
	if err != nil {
		return nil, fmt.Errorf("Error getting QueryType from passed array %s", err)
	}

	query := Query{context: ctx, array: array}
	ret := C.tiledb_query_alloc(query.context.tiledbContext, array.tiledbArray, C.tiledb_query_type_t(queryType), &query.tiledbQuery)
	if ret != C.TILEDB_OK {
		return nil, fmt.Errorf("Error creating tiledb query: %s", query.context.LastError())
	}

	// Set finalizer for free C pointer on gc
	runtime.SetFinalizer(&query, func(query *Query) {
		query.Free()
	})

	query.resultBufferElements = make(map[string][2]*uint64, 0)

	return &query, nil
}

// Free tiledb_query_t that was allocated on heap in c
func (q *Query) Free() {
	q.bufferMutex.Lock()
	defer q.bufferMutex.Unlock()
	q.buffers = nil
	q.resultBufferElements = nil
	if q.tiledbQuery != nil {
		C.tiledb_query_free(&q.tiledbQuery)
	}
}

// SetSubArray Sets a subarray, defined in the order dimensions were added.
// Coordinates are inclusive. For the case of writes, this is meaningful only
// for dense arrays, and specifically dense writes.
func (q *Query) SetSubArray(subArray interface{}) error {

	if reflect.TypeOf(subArray).Kind() != reflect.Slice {
		return fmt.Errorf("Subarray passed must be a slice, type passed was: %s", reflect.TypeOf(subArray).Kind().String())
	}

	subArrayType := reflect.TypeOf(subArray).Elem().Kind()

	schema, err := q.array.Schema()
	if err != nil {
		return fmt.Errorf("Could not get array schema from query array: %s", err)
	}

	domain, err := schema.Domain()
	if err != nil {
		return fmt.Errorf("Could not get domain from array schema: %s", err)
	}

	domainType, err := domain.Type()
	if err != nil {
		return fmt.Errorf("Could not get domain type: %s", err)
	}

	if subArrayType != domainType.ReflectKind() {
		return fmt.Errorf("Domain and subarray do not have the same data types. Domain: %s, Extent: %s", domainType.ReflectKind().String(), subArrayType.String())
	}

	var csubArray unsafe.Pointer
	switch subArrayType {
	case reflect.Int:
		// Create subArray void*
		tmpSubArray := subArray.([]int)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Int8:
		// Create subArray void*
		tmpSubArray := subArray.([]int8)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Int16:
		// Create subArray void*
		tmpSubArray := subArray.([]int16)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Int32:
		// Create subArray void*
		tmpSubArray := subArray.([]int32)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Int64:
		// Create subArray void*
		tmpSubArray := subArray.([]int64)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Uint:
		// Create subArray void*
		tmpSubArray := subArray.([]uint)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Uint8:
		// Create subArray void*
		tmpSubArray := subArray.([]uint8)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Uint16:
		// Create subArray void*
		tmpSubArray := subArray.([]uint16)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Uint32:
		// Create subArray void*
		tmpSubArray := subArray.([]uint32)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Uint64:
		// Create subArray void*
		tmpSubArray := subArray.([]uint64)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Float32:
		// Create subArray void*
		tmpSubArray := subArray.([]float32)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	case reflect.Float64:
		// Create subArray void*
		tmpSubArray := subArray.([]float64)
		csubArray = unsafe.Pointer(&tmpSubArray[0])
	default:
		return fmt.Errorf("Unrecognized subArray type passed: %s", subArrayType.String())
	}

	ret := C.tiledb_query_set_subarray(q.context.tiledbContext, q.tiledbQuery, csubArray)
	if ret != C.TILEDB_OK {
		return fmt.Errorf("Error setting query subarray: %s", q.context.LastError())
	}
	return nil
}

// SetBufferUnsafe Sets the buffer for a fixed-sized attribute to a query
// This takes an unsafe pointer which is passsed straight to tiledb c_api
// for advanced useage
func (q *Query) SetBufferUnsafe(attribute string, buffer unsafe.Pointer, bufferSize uint64) (*uint64, error) {
	cAttribute := C.CString(attribute)
	defer C.free(unsafe.Pointer(cAttribute))

	ret := C.tiledb_query_set_buffer(
		q.context.tiledbContext,
		q.tiledbQuery,
		cAttribute,
		buffer,
		(*C.uint64_t)(unsafe.Pointer(&bufferSize)))

	if ret != C.TILEDB_OK {
		return nil, fmt.Errorf(
			"Error setting query buffer: %s", q.context.LastError())
	}

	q.resultBufferElements[attribute] = [2]*uint64{nil, &bufferSize}

	return &bufferSize, nil
}

// SetBuffer Sets the buffer for a fixed-sized attribute to a query
// The buffer must be an initialized slice
func (q *Query) SetBuffer(attributeOrDimension string, buffer interface{}) (*uint64,
	error) {
	bufferReflectType := reflect.TypeOf(buffer)
	bufferReflectValue := reflect.ValueOf(buffer)
	if bufferReflectValue.Kind() != reflect.Slice {
		return nil, fmt.Errorf(
			"Buffer passed must be a slice that is pre"+
				"-allocated, type passed was: %s",
			bufferReflectValue.Kind().String())
	}

	// Next get the attribute to validate the buffer type is the same as the attribute
	schema, err := q.array.Schema()
	if err != nil {
		return nil, fmt.Errorf(
			"Could not get array schema for SetBuffer: %s",
			err)
	}

	domain, err := schema.Domain()
	if err != nil {
		return nil, fmt.Errorf(
			"Could not get domain for SetBuffer: %s",
			attributeOrDimension)
	}

	var attributeOrDimensionType Datatype
	// If we are setting tiledb coordinates for a sparse array we want to check
	// the domain type. The TILEDB_COORDS attribute is only materialized after
	// the first write
	if attributeOrDimension == TILEDB_COORDS {
		attributeOrDimensionType, err = domain.Type()
		if err != nil {
			return nil, fmt.Errorf(
				"Could not get domainType for SetBuffer: %s",
				attributeOrDimension)
		}
	} else {
		hasDim, err := domain.HasDimension(attributeOrDimension)
		if err != nil {
			return nil, err
		}

		if hasDim {
			dimension, err := domain.DimensionFromName(attributeOrDimension)
			if err != nil {
				return nil, fmt.Errorf("Could not get attribute or dimension for SetBuffer: %s",
					attributeOrDimension)
			}

			attributeOrDimensionType, err = dimension.Type()
			if err != nil {
				return nil, fmt.Errorf("Could not get dimensionType for SetBuffer: %s",
					attributeOrDimension)
			}
		} else {
			schemaAttribute, err := schema.AttributeFromName(attributeOrDimension)
			if err != nil {
				return nil, fmt.Errorf("Could not get attribute %s for SetBuffer",
					attributeOrDimension)
			}

			attributeOrDimensionType, err = schemaAttribute.Type()
			if err != nil {
				return nil, fmt.Errorf("Could not get attributeType for SetBuffer: %s",
					attributeOrDimension)
			}
		}
	}

	bufferType := bufferReflectType.Elem().Kind()
	if attributeOrDimensionType.ReflectKind() != bufferType {
		return nil, fmt.Errorf("Buffer and Attribute do not have the same"+
			" data types. Buffer: %s, Attribute: %s",
			bufferType.String(),
			attributeOrDimensionType.ReflectKind().String())
	}

	var cbuffer unsafe.Pointer
	// Get length of slice, this will be multiplied by size of datatype below
	bufferSize := uint64(bufferReflectValue.Len())

	if bufferSize == uint64(0) {
		return nil, fmt.Errorf(
			"Buffer has no length, vbuffers are required to be " +
				"initialized before reading or writting")
	}

	// Acquire a lock to make appending to buffer slice thread safe
	q.bufferMutex.Lock()
	defer q.bufferMutex.Unlock()

	switch bufferType {
	case reflect.Int:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]int)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Int8:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int8(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]int8)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Int16:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int16(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]int16)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Int32:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int32(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]int32)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Int64:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int64(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]int64)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]uint)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint8:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint8(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]uint8)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint16:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint16(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]uint16)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint32:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint32(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]uint32)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint64:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint64(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]uint64)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Float32:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(float32(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]float32)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Float64:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(float64(0)))
		// Create buffer void*
		tmpBuffer := buffer.([]float64)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	default:
		return nil,
			fmt.Errorf("Unrecognized buffer type passed: %s",
				bufferType.String())
	}

	cAttributeOrDimension := C.CString(attributeOrDimension)
	defer C.free(unsafe.Pointer(cAttributeOrDimension))

	ret := C.tiledb_query_set_buffer(
		q.context.tiledbContext,
		q.tiledbQuery,
		cAttributeOrDimension,
		cbuffer,
		(*C.uint64_t)(unsafe.Pointer(&bufferSize)))

	if ret != C.TILEDB_OK {
		return nil, fmt.Errorf(
			"Error setting query buffer: %s", q.context.LastError())
	}

	q.resultBufferElements[attributeOrDimension] =
		[2]*uint64{nil, &bufferSize}

	return &bufferSize, nil
}

// AddRange adds a 1D range along a subarray dimension, which is in the form
// (start, end, stride). The datatype of the range components must be the same
// as the type of the domain of the array in the query.
// The stride is currently unsupported and set to nil.
func (q *Query) AddRange(dimIdx uint32, start interface{}, end interface{}) error {
	startReflectValue := reflect.ValueOf(start)
	endReflectValue := reflect.ValueOf(end)

	if startReflectValue.Kind() != endReflectValue.Kind() {
		return fmt.Errorf(
			"The datatype of the range components must be the same as the type, start was: %s, end was: %s",
			startReflectValue.Kind().String(), endReflectValue.Kind().String())
	}

	var startBuffer unsafe.Pointer
	var endBuffer unsafe.Pointer

	startReflectType := reflect.TypeOf(start)
	startType := startReflectType.Kind()

	switch startType {
	case reflect.Int:
		tStart := start.(int)
		tEnd := end.(int)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Int8:
		tStart := start.(int8)
		tEnd := end.(int8)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Int16:
		tStart := start.(int16)
		tEnd := end.(int16)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Int32:
		tStart := start.(int32)
		tEnd := end.(int32)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Int64:
		tStart := start.(int64)
		tEnd := end.(int64)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Uint:
		tStart := start.(uint)
		tEnd := end.(uint)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Uint8:
		tStart := start.(uint8)
		tEnd := end.(uint8)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Uint16:
		tStart := start.(uint16)
		tEnd := end.(uint16)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Uint32:
		tStart := start.(uint32)
		tEnd := end.(uint32)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Uint64:
		tStart := start.(uint64)
		tEnd := end.(uint64)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Float32:
		tStart := start.(float32)
		tEnd := end.(float32)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	case reflect.Float64:
		tStart := start.(float64)
		tEnd := end.(float64)
		startBuffer = unsafe.Pointer(&tStart)
		endBuffer = unsafe.Pointer(&tEnd)
	default:
		return fmt.Errorf("Unrecognized type of range component passed: %s",
			startType.String())
	}

	ret := C.tiledb_query_add_range(
		q.context.tiledbContext, q.tiledbQuery,
		(C.uint32_t)(dimIdx), startBuffer, endBuffer, nil)

	if ret != C.TILEDB_OK {
		return fmt.Errorf(
			"Error adding query range: %s", q.context.LastError())
	}

	return nil
}

// AddRangeVar adds a range applicable to variable-sized dimensions
// Applicable only to string dimensions
func (q *Query) AddRangeVar(dimIdx uint32, start interface{}, end interface{}) error {
	startReflectValue := reflect.ValueOf(start)
	endReflectValue := reflect.ValueOf(end)

	if startReflectValue.Kind() != reflect.Slice {
		return fmt.Errorf("Start buffer passed must be a slice that is pre"+
			"-allocated, type passed was: %s", startReflectValue.Kind().String())
	}

	if endReflectValue.Kind() != reflect.Slice {
		return fmt.Errorf("End buffer passed must be a slice that is pre"+
			"-allocated, type passed was: %s", endReflectValue.Kind().String())
	}

	startSize := uint64(startReflectValue.Len())
	endSize := uint64(endReflectValue.Len())

	var startBuffer unsafe.Pointer
	var endBuffer unsafe.Pointer

	startReflectType := reflect.TypeOf(start)
	startType := startReflectType.Elem().Kind()

	switch startType {
	case reflect.Int:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Int8:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Int16:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Int32:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Int64:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Uint:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Uint8:
		tStart := start.([]uint8)
		tEnd := end.([]uint8)
		startBuffer = unsafe.Pointer(&(tStart[0]))
		endBuffer = unsafe.Pointer(&(tEnd[0]))

		ret := C.tiledb_query_add_range_var(
			q.context.tiledbContext, q.tiledbQuery,
			(C.uint32_t)(dimIdx), startBuffer, (C.uint64_t)(startSize), endBuffer, (C.uint64_t)(endSize))

		if ret != C.TILEDB_OK {
			return fmt.Errorf(
				"Error adding query range var: %s", q.context.LastError())
		}
	case reflect.Uint16:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Uint32:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Uint64:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Float32:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	case reflect.Float64:
		return fmt.Errorf("Unsupported type of range component passed: %s",
			startType.String())
	default:
		return fmt.Errorf("Unrecognized type of range component passed: %s",
			startType.String())
	}

	return nil
}

// GetRange retrieves a specific range of the query subarray
// along a given dimension.
// Returns (start, end, error)
// Stride is not supported at the moment, always nil
func (q *Query) GetRange(dimIdx uint32, rangeNum uint64) (interface{}, interface{}, error) {
	var pStart, pEnd, pStride unsafe.Pointer

	// Based on the type we fill in the interface{} objects for start, end
	var start, end interface{}

	// We need to infer the datatype of the dimension represented by index
	// dimIdx. That said:
	// Get array schema
	schema, err := q.array.Schema()
	if err != nil {
		return nil, nil, err
	}

	// Get the domain object
	domain, err := schema.Domain()
	if err != nil {
		return nil, nil, err
	}

	// Use the index to retrieve the dimension object
	dimension, err := domain.DimensionFromIndex(uint(dimIdx))
	if err != nil {
		return nil, nil, err
	}

	// Finally get the dimension's type
	datatype, err := dimension.Type()
	if err != nil {
		return nil, nil, err
	}

	cellValNum, err := dimension.CellValNum()
	if err != nil {
		return nil, nil, err
	}

	if cellValNum == TILEDB_VAR_NUM {

		var startSize, endSize C.uint64_t

		ret := C.tiledb_query_get_range_var_size(
			q.context.tiledbContext, q.tiledbQuery,
			(C.uint32_t)(dimIdx), (C.uint64_t)(rangeNum), &startSize, &endSize)

		if ret != C.TILEDB_OK {
			return nil, nil, fmt.Errorf(
				"Error retrieving query range: %s", q.context.LastError())
		}

		startData := make([]byte, startSize)
		endData := make([]byte, endSize)

		ret = C.tiledb_query_get_range_var(
			q.context.tiledbContext, q.tiledbQuery,
			(C.uint32_t)(dimIdx), (C.uint64_t)(rangeNum), unsafe.Pointer(&startData[0]), unsafe.Pointer(&endData[0]))

		if ret != C.TILEDB_OK {
			return nil, nil, fmt.Errorf(
				"Error retrieving query range: %s", q.context.LastError())
		}

		start = startData
		end = endData

	} else {
		ret := C.tiledb_query_get_range(
			q.context.tiledbContext, q.tiledbQuery,
			(C.uint32_t)(dimIdx), (C.uint64_t)(rangeNum), &pStart, &pEnd, &pStride)

		if ret != C.TILEDB_OK {
			return nil, nil, fmt.Errorf(
				"Error retrieving query range: %s", q.context.LastError())
		}

		switch datatype {
		case TILEDB_INT8:
			start = *(*int8)(unsafe.Pointer(pStart))
			end = *(*int8)(unsafe.Pointer(pEnd))
		case TILEDB_INT16:
			start = *(*int16)(unsafe.Pointer(pStart))
			end = *(*int16)(unsafe.Pointer(pEnd))
		case TILEDB_INT32:
			start = *(*int32)(unsafe.Pointer(pStart))
			end = *(*int32)(unsafe.Pointer(pEnd))
		case TILEDB_INT64, TILEDB_DATETIME_YEAR, TILEDB_DATETIME_MONTH, TILEDB_DATETIME_WEEK, TILEDB_DATETIME_DAY, TILEDB_DATETIME_HR, TILEDB_DATETIME_MIN, TILEDB_DATETIME_SEC, TILEDB_DATETIME_MS, TILEDB_DATETIME_US, TILEDB_DATETIME_NS, TILEDB_DATETIME_PS, TILEDB_DATETIME_FS, TILEDB_DATETIME_AS:
			start = *(*int64)(unsafe.Pointer(pStart))
			end = *(*int64)(unsafe.Pointer(pEnd))
		case TILEDB_UINT8:
			start = *(*uint8)(unsafe.Pointer(pStart))
			end = *(*uint8)(unsafe.Pointer(pEnd))
		case TILEDB_UINT16:
			start = *(*uint16)(unsafe.Pointer(pStart))
			end = *(*uint16)(unsafe.Pointer(pEnd))
		case TILEDB_UINT32:
			start = *(*uint32)(unsafe.Pointer(pStart))
			end = *(*uint32)(unsafe.Pointer(pEnd))
		case TILEDB_UINT64:
			start = *(*uint64)(unsafe.Pointer(pStart))
			end = *(*uint64)(unsafe.Pointer(pEnd))
		case TILEDB_FLOAT32:
			start = *(*float32)(unsafe.Pointer(pStart))
			end = *(*float32)(unsafe.Pointer(pEnd))
		case TILEDB_FLOAT64:
			start = *(*float64)(unsafe.Pointer(pStart))
			end = *(*float64)(unsafe.Pointer(pEnd))
		case TILEDB_STRING_ASCII:
			start = *(*uint8)(unsafe.Pointer(pStart))
			end = *(*uint8)(unsafe.Pointer(pEnd))
		default:
			return nil, nil, fmt.Errorf("Unrecognized dimension type: %d", datatype)
		}
	}

	return start, end, nil
}

// GetRangeVar exists for continuinity with other TileDB APIs
// GetRange in Golang supports the variable length attribute also
// The function retrieves a specific range of the query subarray
// along a given dimension.
// Returns (start, end, error)
func (q *Query) GetRangeVar(dimIdx uint32, rangeNum uint64) (interface{}, interface{}, error) {
	return q.GetRange(dimIdx, rangeNum)
}

// GetRanges gets the number of dimensions from the array under current query
// and builds an array of dimensions that have as memmbers arrays of ranges
func (q *Query) GetRanges() (map[string][]RangeLimits, error) {
	// We need to infer the datatype of the dimension represented by index
	// dimIdx. That said:
	// Get array schema
	schema, err := q.array.Schema()
	if err != nil {
		return nil, err
	}

	// Get the domain object
	domain, err := schema.Domain()
	if err != nil {
		return nil, err
	}

	// Use the index to retrieve the dimension object
	nDim, err := domain.NDim()
	if err != nil {
		return nil, err
	}

	var dimIdx uint

	rangeMap := make(map[string][]RangeLimits)
	for dimIdx = 0; dimIdx < nDim; dimIdx++ {
		// Get dimension object
		dimension, err := domain.DimensionFromIndex(dimIdx)
		if err != nil {
			return nil, err
		}
		// Get name from dimension
		name, err := dimension.Name()
		if err != nil {
			return nil, err
		}

		// Get number of renges to iterate
		numOfRanges, err := q.GetRangeNum(uint32(dimIdx))
		if err != nil {
			return nil, err
		}

		var I uint64
		rangeArray := make([]RangeLimits, 0)
		for I = 0; I < *numOfRanges; I++ {

			start, end, err := q.GetRange(uint32(dimIdx), I)
			if err != nil {
				return nil, err
			}
			// Append range to range Array
			rangeArray = append(rangeArray, RangeLimits{start: start, end: end})
		}
		// key: name (string), value: rangeArray ([]RangeLimits)
		rangeMap[name] = rangeArray
	}

	return rangeMap, err
}

// GetRangeNum retrieves the number of ranges of the query subarray
// along a given dimension.
func (q *Query) GetRangeNum(dimIdx uint32) (*uint64, error) {
	var rangeNum uint64

	ret := C.tiledb_query_get_range_num(
		q.context.tiledbContext, q.tiledbQuery,
		(C.uint32_t)(dimIdx), (*C.uint64_t)(unsafe.Pointer(&rangeNum)))

	if ret != C.TILEDB_OK {
		return nil, fmt.Errorf(
			"Error retrieving query range num: %s", q.context.LastError())
	}

	return &rangeNum, nil
}

// Buffer returns a slice backed by the underlying c buffer from tiledb
func (q *Query) Buffer(attributeOrDimension string) (interface{}, error) {
	var datatype Datatype
	schema, err := q.array.Schema()
	if err != nil {
		return nil, err
	}

	domain, err := schema.Domain()
	if err != nil {
		return nil, fmt.Errorf(
			"Could not get domain from array schema for Buffer: %s",
			err)
	}

	if attributeOrDimension == TILEDB_COORDS {
		datatype, err = domain.Type()
		if err != nil {
			return nil, err
		}
	} else {
		hasDim, err := domain.HasDimension(attributeOrDimension)
		if err != nil {
			return nil, err
		}

		if hasDim {
			dimension, err := domain.DimensionFromName(attributeOrDimension)
			if err != nil {
				return nil, fmt.Errorf("Could not get attribute or dimension for SetBuffer: %s", attributeOrDimension)
			}

			datatype, err = dimension.Type()
			if err != nil {
				return nil, fmt.Errorf("Could not get dimensionType for SetBuffer: %s", attributeOrDimension)
			}
		} else {
			attribute, err := schema.AttributeFromName(attributeOrDimension)
			if err != nil {
				return nil, fmt.Errorf("Could not get attribute %s for Buffer", attributeOrDimension)
			}

			datatype, err = attribute.Type()
			if err != nil {
				return nil, fmt.Errorf("Could not get attributeType for SetBuffer: %s", attributeOrDimension)
			}
		}
	}

	cAttributeOrDimension := C.CString(attributeOrDimension)
	defer C.free(unsafe.Pointer(cAttributeOrDimension))

	var ret C.int32_t
	var cbufferSize *C.uint64_t
	var cbuffer unsafe.Pointer
	var buffer interface{}
	switch datatype {
	case TILEDB_INT8:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int8_t
		buffer = (*[1 << 46]int8)(cbuffer)[:length:length]

	case TILEDB_INT16:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int16_t
		buffer = (*[1 << 46]int16)(cbuffer)[:length:length]

	case TILEDB_INT32:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int32_t
		buffer = (*[1 << 46]int32)(cbuffer)[:length:length]

	case TILEDB_INT64, TILEDB_DATETIME_YEAR, TILEDB_DATETIME_MONTH, TILEDB_DATETIME_WEEK, TILEDB_DATETIME_DAY, TILEDB_DATETIME_HR, TILEDB_DATETIME_MIN, TILEDB_DATETIME_SEC, TILEDB_DATETIME_MS, TILEDB_DATETIME_US, TILEDB_DATETIME_NS, TILEDB_DATETIME_PS, TILEDB_DATETIME_FS, TILEDB_DATETIME_AS:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int64_t
		buffer = (*[1 << 46]int64)(cbuffer)[:length:length]

	case TILEDB_UINT8:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint8_t
		buffer = (*[1 << 46]uint8)(cbuffer)[:length:length]

	case TILEDB_UINT16:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint16_t
		buffer = (*[1 << 46]uint16)(cbuffer)[:length:length]

	case TILEDB_UINT32:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint32_t
		buffer = (*[1 << 46]uint32)(cbuffer)[:length:length]

	case TILEDB_UINT64:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint64_t
		buffer = (*[1 << 46]uint64)(cbuffer)[:length:length]

	case TILEDB_FLOAT32:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_float
		buffer = (*[1 << 46]float32)(cbuffer)[:length:length]

	case TILEDB_FLOAT64:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_double
		buffer = (*[1 << 46]float64)(cbuffer)[:length:length]

	case TILEDB_CHAR:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_char
		buffer = (*[1 << 46]byte)(cbuffer)[:length:length]

	case TILEDB_STRING_ASCII:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint8_t
		buffer = (*[1 << 46]uint8)(cbuffer)[:length:length]

	case TILEDB_STRING_UTF8:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint8_t
		buffer = (*[1 << 46]uint8)(cbuffer)[:length:length]

	case TILEDB_STRING_UTF16:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint16_t
		buffer = (*[1 << 46]uint16)(cbuffer)[:length:length]

	case TILEDB_STRING_UTF32:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint32_t
		buffer = (*[1 << 46]uint32)(cbuffer)[:length:length]

	case TILEDB_STRING_UCS2:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint16_t
		buffer = (*[1 << 46]uint16)(cbuffer)[:length:length]

	case TILEDB_STRING_UCS4:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint32_t
		buffer = (*[1 << 46]uint32)(cbuffer)[:length:length]

	case TILEDB_ANY:
		ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cAttributeOrDimension, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int32_t
		buffer = (*[1 << 46]C.int8_t)(cbuffer)[:length:length]

	default:
		return nil, fmt.Errorf("Unrecognized attribute type: %d", datatype)
	}
	if ret != C.TILEDB_OK {
		return nil, fmt.Errorf("Error getting tiledb query buffer for %s: %s", attributeOrDimension, q.context.LastError())
	}

	return buffer, nil
}

// SetBufferVarUnsafe Sets the buffer for a variable sized attribute to a query
// This takes unsafe pointers which is passsed straight to tiledb c_api
// for advanced useage
func (q *Query) SetBufferVarUnsafe(attribute string, offset unsafe.Pointer, offsetSize uint64, buffer unsafe.Pointer, bufferSize uint64) (*uint64, *uint64, error) {
	cAttribute := C.CString(attribute)
	defer C.free(unsafe.Pointer(cAttribute))

	ret := C.tiledb_query_set_buffer_var(
		q.context.tiledbContext,
		q.tiledbQuery,
		cAttribute,
		(*C.uint64_t)(offset),
		(*C.uint64_t)(unsafe.Pointer(&offsetSize)),
		buffer,
		(*C.uint64_t)(unsafe.Pointer(&bufferSize)))

	if ret != C.TILEDB_OK {
		return nil, nil, fmt.Errorf("Error setting query var buffer: %s", q.context.LastError())
	}

	q.resultBufferElements[attribute] = [2]*uint64{&offsetSize, &bufferSize}

	return &offsetSize, &bufferSize, nil
}

// SetBufferVar Sets the buffer for a variable sized attribute/dimension to a query
// The buffer must be an initialized slice
func (q *Query) SetBufferVar(attributeOrDimension string, offset []uint64, buffer interface{}) (*uint64, *uint64, error) {
	bufferReflectType := reflect.TypeOf(buffer)
	bufferReflectValue := reflect.ValueOf(buffer)
	if bufferReflectValue.Kind() != reflect.Slice {
		return nil, nil, fmt.Errorf("Buffer passed must be a slice that is pre"+
			"-allocated, type passed was: %s", bufferReflectValue.Kind().String())
	}

	// Next get the attribute to validate the buffer type is the same as the attribute
	schema, err := q.array.Schema()
	if err != nil {
		return nil, nil, fmt.Errorf(
			"Could not get array schema for SetBuffer: %s",
			err)
	}

	var attributeOrDimensionType Datatype

	domain, err := schema.Domain()
	if err != nil {
		return nil, nil, fmt.Errorf(
			"Could not get domain from array schema for SetBufferVar: %s",
			err)
	}

	hasDim, err := domain.HasDimension(attributeOrDimension)
	if err != nil {
		return nil, nil, err
	}

	if hasDim {
		dimension, err := domain.DimensionFromName(attributeOrDimension)
		if err != nil {
			return nil, nil, fmt.Errorf("Could not get attribute or dimension for SetBufferVar: %s",
				attributeOrDimension)
		}
		attributeOrDimensionType, err = dimension.Type()
		if err != nil {
			return nil, nil, fmt.Errorf("Could not get dimensionType for SetBufferVar: %s",
				attributeOrDimension)
		}
	} else {
		schemaAttribute, err := schema.AttributeFromName(attributeOrDimension)
		if err != nil {
			return nil, nil, fmt.Errorf("Could not get attribute %s SetBufferVar",
				attributeOrDimension)
		}

		attributeOrDimensionType, err = schemaAttribute.Type()
		if err != nil {
			return nil, nil, fmt.Errorf("Could not get attributeType for SetBufferVar: %s",
				attributeOrDimension)
		}
	}

	bufferType := bufferReflectType.Elem().Kind()

	if attributeOrDimensionType.ReflectKind() != bufferType {
		return nil, nil, fmt.Errorf("Buffer and Attribute do not have the same"+
			" data types. Buffer: %s, Attribute: %s", bufferType.String(), attributeOrDimensionType.ReflectKind().String())
	}

	bufferSize := uint64(bufferReflectValue.Len())

	if bufferSize == uint64(0) {
		return nil, nil, fmt.Errorf("Buffer has no length, " +
			"buffers are required to be initialized before reading or writting")
	}

	offsetSize := uint64(len(offset)) * uint64(unsafe.Sizeof(uint64(0)))

	if offsetSize == uint64(0) {
		return nil, nil, fmt.Errorf("Offset slice has no length, " +
			"offset slices are required to be initialized before reading or writting")
	}

	// Acquire a lock to make appending to buffer slice thread safe
	q.bufferMutex.Lock()
	defer q.bufferMutex.Unlock()

	// Store offset so array does not get gc'ed
	q.buffers = append(q.buffers, offset)

	// Set offset and buffer
	var cbuffer unsafe.Pointer
	coffset := unsafe.Pointer(&(offset)[0])
	switch bufferType {
	case reflect.Int:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]int)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Int8:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int8(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]int8)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Int16:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int16(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]int16)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Int32:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int32(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]int32)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Int64:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(int64(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]int64)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]uint)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint8:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint8(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]uint8)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint16:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint16(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]uint16)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint32:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint32(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]uint32)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Uint64:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(uint64(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]uint64)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Float32:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(float32(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]float32)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	case reflect.Float64:
		// Set buffersize
		bufferSize = bufferSize * uint64(unsafe.Sizeof(float64(0)))

		// Create buffer void*
		tmpBuffer := buffer.([]float64)
		// Store slice so underlying array is not gc'ed
		q.buffers = append(q.buffers, tmpBuffer)
		cbuffer = unsafe.Pointer(&(tmpBuffer)[0])
	default:
		return nil, nil, fmt.Errorf("Unrecognized buffer type passed: %s",
			bufferType.String())
	}

	cAttributeOrDimension := C.CString(attributeOrDimension)
	defer C.free(unsafe.Pointer(cAttributeOrDimension))

	ret := C.tiledb_query_set_buffer_var(
		q.context.tiledbContext,
		q.tiledbQuery,
		cAttributeOrDimension,
		(*C.uint64_t)(coffset),
		(*C.uint64_t)(unsafe.Pointer(&offsetSize)),
		cbuffer,
		(*C.uint64_t)(unsafe.Pointer(&bufferSize)))

	if ret != C.TILEDB_OK {
		return nil, nil, fmt.Errorf("Error setting query var buffer: %s",
			q.context.LastError())
	}

	q.resultBufferElements[attributeOrDimension] =
		[2]*uint64{&offsetSize, &bufferSize}

	return &offsetSize, &bufferSize, nil
}

// ResultBufferElements returns the number of elements in the result buffers
// from a read query.
// This is a map from the attribute name to a pair of values.
// The first is number of elements (offsets) for var size attributes, and the
// second is number of elements in the data buffer. For fixed sized attributes
// (and coordinates), the first is always 0.
func (q *Query) ResultBufferElements() (map[string][2]uint64, error) {
	elements := make(map[string][2]uint64, 0)

	// Will need the schema to infer data type size for attributes
	schema, err := q.array.Schema()
	if err != nil {
		return nil, fmt.Errorf("Could not get schema for ResultBufferElements: %s", err)
	}

	domain, err := schema.Domain()
	if err != nil {
		return nil, fmt.Errorf("Could not get domain for ResultBufferElements: %s", err)
	}

	var datatype Datatype
	for attributeOrDimension, v := range q.resultBufferElements {
		// Handle coordinates
		if attributeOrDimension == TILEDB_COORDS {
			// For fixed length attributes offset elements are always zero
			offsetElements := uint64(0)

			domainType, err := domain.Type()
			if err != nil {
				return nil, fmt.Errorf("Could not get domainType for ResultBufferElements: %s", err)
			}

			// Number of buffer elements is calculated
			bufferElements := (*v[1]) / domainType.Size()
			elements[attributeOrDimension] = [2]uint64{offsetElements, bufferElements}
		} else {
			// For fixed length attributes offset elements are always zero
			offsetElements := uint64(0)
			if v[0] != nil {
				// The attribute is variable lenght
				offsetElements = (*v[0]) / uint64(unsafe.Sizeof(uint64(0)))
			}

			hasDim, err := domain.HasDimension(attributeOrDimension)
			if err != nil {
				return nil, err
			}

			if hasDim {
				dimension, err := domain.DimensionFromName(attributeOrDimension)
				if err != nil {
					return nil, fmt.Errorf("Could not get attribute or dimension for SetBuffer: %s", attributeOrDimension)
				}

				datatype, err = dimension.Type()
				if err != nil {
					return nil, fmt.Errorf("Could not get dimensionType for SetBuffer: %s", attributeOrDimension)
				}
			} else {
				// Get the attribute
				attribute, err := schema.AttributeFromName(attributeOrDimension)
				if err != nil {
					return nil, fmt.Errorf("Could not get attribute %s for ResultBufferElements: %s", attributeOrDimension, err)
				}

				// Get datatype size to convert byte lengths to needed buffer sizes
				datatype, err = attribute.Type()
				if err != nil {
					return nil, fmt.Errorf("Could not get attribute type for ResultBufferElements: %s", err)
				}
			}

			// Number of buffer elements is calculated
			bufferElements := (*v[1]) / datatype.Size()
			elements[attributeOrDimension] = [2]uint64{offsetElements, bufferElements}
		}
	}

	return elements, nil
}

// BufferVar returns a slice backed by the underlying c buffer from tiledb for
// offets and values
func (q *Query) BufferVar(attributeOrDimension string) ([]uint64, interface{}, error) {
	var datatype Datatype
	schema, err := q.array.Schema()
	if err != nil {
		return nil, nil, err
	}

	domain, err := schema.Domain()
	if err != nil {
		return nil, nil, fmt.Errorf(
			"Could not get domain from array schema for BufferVar: %s",
			err)
	}

	if attributeOrDimension == TILEDB_COORDS {
		datatype, err = domain.Type()
		if err != nil {
			return nil, nil, err
		}
	} else {
		hasDim, err := domain.HasDimension(attributeOrDimension)
		if err != nil {
			return nil, nil, err
		}

		if hasDim {
			dimension, err := domain.DimensionFromName(attributeOrDimension)
			if err != nil {
				return nil, nil, fmt.Errorf("Could not get attribute or dimension for BufferVar: %s", attributeOrDimension)
			}

			datatype, err = dimension.Type()
			if err != nil {
				return nil, nil, fmt.Errorf("Could not get dimensionType for BufferVar: %s", attributeOrDimension)
			}
		} else {
			attribute, err := schema.AttributeFromName(attributeOrDimension)
			if err != nil {
				return nil, nil, fmt.Errorf("Could not get attribute for BufferVar: %s", attributeOrDimension)
			}

			datatype, err = attribute.Type()
			if err != nil {
				return nil, nil, fmt.Errorf("Could not get attributeType for BufferVar: %s", attributeOrDimension)
			}
		}
	}

	cattributeNameOrDimension := C.CString(attributeOrDimension)
	defer C.free(unsafe.Pointer(cattributeNameOrDimension))

	var ret C.int32_t
	var cbufferSize *C.uint64_t
	var cbuffer unsafe.Pointer
	var buffer interface{}
	var coffsetsSize *C.uint64_t
	var coffsets *C.uint64_t
	var offsets []uint64
	switch datatype {
	case TILEDB_INT8:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int8_t
		buffer = (*[1 << 46]int8)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_INT16:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int16_t
		buffer = (*[1 << 46]int16)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_INT32:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int32_t
		buffer = (*[1 << 46]int32)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_INT64, TILEDB_DATETIME_YEAR, TILEDB_DATETIME_MONTH, TILEDB_DATETIME_WEEK, TILEDB_DATETIME_DAY, TILEDB_DATETIME_HR, TILEDB_DATETIME_MIN, TILEDB_DATETIME_SEC, TILEDB_DATETIME_MS, TILEDB_DATETIME_US, TILEDB_DATETIME_NS, TILEDB_DATETIME_PS, TILEDB_DATETIME_FS, TILEDB_DATETIME_AS:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int64_t
		buffer = (*[1 << 46]int64)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_UINT8:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint8_t
		buffer = (*[1 << 46]uint8)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_UINT16:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint16_t
		buffer = (*[1 << 46]uint16)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_UINT32:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint32_t
		buffer = (*[1 << 46]uint32)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_UINT64:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint64_t
		buffer = (*[1 << 46]uint64)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_FLOAT32:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_float
		buffer = (*[1 << 46]float32)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_FLOAT64:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_double
		buffer = (*[1 << 46]float64)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_CHAR:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_char
		buffer = (*[1 << 46]byte)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_STRING_ASCII:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint8_t
		buffer = (*[1 << 46]uint8)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_STRING_UTF8:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint8_t
		buffer = (*[1 << 46]uint8)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_STRING_UTF16:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint16_t
		buffer = (*[1 << 46]uint16)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_STRING_UTF32:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint32_t
		buffer = (*[1 << 46]uint32)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_STRING_UCS2:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint16_t
		buffer = (*[1 << 46]uint16)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_STRING_UCS4:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_uint32_t
		buffer = (*[1 << 46]uint32)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	case TILEDB_ANY:
		ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
		length := (*cbufferSize) / C.sizeof_int32_t
		buffer = (*[1 << 46]C.int8_t)(cbuffer)[:length:length]
		offsetsLength := *coffsetsSize / C.sizeof_uint64_t
		offsets = (*[1 << 46]uint64)(unsafe.Pointer(coffsets))[:offsetsLength:offsetsLength]

	default:
		return nil, nil, fmt.Errorf("Unrecognized attribute type: %d", datatype)
	}
	if ret != C.TILEDB_OK {
		return nil, nil, fmt.Errorf("Error getting tiledb query buffer for %s: %s", attributeOrDimension, q.context.LastError())
	}

	return offsets, buffer, nil
}

// BufferSizeVar returns the size (in num elements) of the backing C buffers for the given variable-length attribute
func (q *Query) BufferSizeVar(attributeOrDimension string) (uint64, uint64, error) {
	var datatype Datatype
	schema, err := q.array.Schema()
	if err != nil {
		return 0, 0, err
	}

	domain, err := schema.Domain()
	if err != nil {
		return 0, 0, fmt.Errorf(
			"Could not get domain from array schema for BufferSizeVar: %s",
			err)
	}

	if attributeOrDimension == TILEDB_COORDS {
		datatype, err = domain.Type()
		if err != nil {
			return 0, 0, err
		}
	} else {
		hasDim, err := domain.HasDimension(attributeOrDimension)
		if err != nil {
			return 0, 0, err
		}

		if hasDim {
			dimension, err := domain.DimensionFromName(attributeOrDimension)
			if err != nil {
				return 0, 0, fmt.Errorf("Could not get attribute or dimension for BufferSizeVar: %s", attributeOrDimension)
			}

			datatype, err = dimension.Type()
			if err != nil {
				return 0, 0, fmt.Errorf("Could not get dimensionType for BufferSizeVar: %s", attributeOrDimension)
			}
		} else {
			attribute, err := schema.AttributeFromName(attributeOrDimension)
			if err != nil {
				return 0, 0, fmt.Errorf("Could not get attribute %s for BufferSizeVar", attributeOrDimension)
			}

			datatype, err = attribute.Type()
			if err != nil {
				return 0, 0, fmt.Errorf("Could not get attributeType for BufferSizeVar: %s", attributeOrDimension)
			}
		}
	}

	dataTypeSize := datatype.Size()
	offsetTypeSize := TILEDB_UINT64.Size()

	cattributeNameOrDimension := C.CString(attributeOrDimension)
	defer C.free(unsafe.Pointer(cattributeNameOrDimension))

	var ret C.int32_t
	var cbufferSize *C.uint64_t
	var cbuffer unsafe.Pointer
	var coffsetsSize *C.uint64_t
	var coffsets *C.uint64_t
	ret = C.tiledb_query_get_buffer_var(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &coffsets, &coffsetsSize, &cbuffer, &cbufferSize)
	if ret != C.TILEDB_OK {
		return 0, 0, fmt.Errorf("Error getting tiledb query buffer for %s: %s", attributeOrDimension, q.context.LastError())
	}

	var offsetNumElements uint64
	if coffsetsSize == nil {
		offsetNumElements = 0
	} else {
		offsetNumElements = uint64(*coffsetsSize) / offsetTypeSize
	}

	var dataNumElements uint64
	if cbufferSize == nil {
		dataNumElements = 0
	} else {
		dataNumElements = uint64(*cbufferSize) / dataTypeSize
	}

	return offsetNumElements, dataNumElements, nil
}

// BufferSize returns the size (in num elements) of the backing C buffer for the given attribute
func (q *Query) BufferSize(attributeNameOrDimension string) (uint64, error) {
	var datatype Datatype
	schema, err := q.array.Schema()
	if err != nil {
		return 0, err
	}

	domain, err := schema.Domain()
	if err != nil {
		return 0, fmt.Errorf(
			"Could not get domain from array schema for BufferSize: %s",
			err)
	}

	if attributeNameOrDimension == TILEDB_COORDS {
		datatype, err = domain.Type()
		if err != nil {
			return 0, err
		}
	} else {
		hasDim, err := domain.HasDimension(attributeNameOrDimension)
		if err != nil {
			return 0, err
		}

		if hasDim {
			dimension, err := domain.DimensionFromName(attributeNameOrDimension)
			if err != nil {
				return 0, fmt.Errorf("Could not get attribute or dimension for BufferSize: %s", attributeNameOrDimension)
			}

			datatype, err = dimension.Type()
			if err != nil {
				return 0, fmt.Errorf("Could not get dimensionType for BufferSize: %s", attributeNameOrDimension)
			}
		} else {
			attribute, err := schema.AttributeFromName(attributeNameOrDimension)
			if err != nil {
				return 0, err
			}

			datatype, err = attribute.Type()
			if err != nil {
				return 0, err
			}
		}
	}

	dataTypeSize := datatype.Size()

	cattributeNameOrDimension := C.CString(attributeNameOrDimension)
	defer C.free(unsafe.Pointer(cattributeNameOrDimension))

	var ret C.int32_t
	var cbufferSize *C.uint64_t
	var cbuffer unsafe.Pointer
	ret = C.tiledb_query_get_buffer(q.context.tiledbContext, q.tiledbQuery, cattributeNameOrDimension, &cbuffer, &cbufferSize)
	if ret != C.TILEDB_OK {
		return 0, fmt.Errorf("Error getting tiledb query buffer for %s: %s", attributeNameOrDimension, q.context.LastError())
	}

	var dataNumElements uint64
	if cbufferSize == nil {
		dataNumElements = 0
	} else {
		dataNumElements = uint64(*cbufferSize) / dataTypeSize
	}

	return dataNumElements, nil
}

// SetLayout sets the layout of the cells to be written or read
func (q *Query) SetLayout(layout Layout) error {
	ret := C.tiledb_query_set_layout(q.context.tiledbContext, q.tiledbQuery, C.tiledb_layout_t(layout))
	if ret != C.TILEDB_OK {
		return fmt.Errorf("Error setting query layout: %s", q.context.LastError())
	}
	return nil
}

// Finalize Flushes all internal state of a query object and finalizes the
// query. This is applicable only to global layout writes. It has no effect
// for any other query type.
func (q *Query) Finalize() error {
	ret := C.tiledb_query_finalize(q.context.tiledbContext, q.tiledbQuery)
	if ret != C.TILEDB_OK {
		return fmt.Errorf("Error finalizing query: %s", q.context.LastError())
	}
	q.bufferMutex.Lock()
	defer q.bufferMutex.Unlock()
	q.buffers = nil
	return nil
}

/*
Submit a TileDB query
This will block until query is completed

Note:
Finalize() must be invoked after finish writing in global layout
(via repeated invocations of Submit()), in order to flush any internal state.
For the case of reads, if the returned status is TILEDB_INCOMPLETE, TileDB
could not fit the entire result in the user’s buffers. In this case, the user
should consume the read results (if any), optionally reset the buffers with
SetBuffer(), and then resubmit the query until the status becomes
TILEDB_COMPLETED. If all buffer sizes after the termination of this
function become 0, then this means that no useful data was read into
the buffers, implying that the larger buffers are needed for the query
to proceed. In this case, the users must reallocate their buffers
(increasing their size), reset the buffers with set_buffer(),
and resubmit the query.

*/
func (q *Query) Submit() error {
	ret := C.tiledb_query_submit(q.context.tiledbContext, q.tiledbQuery)
	if ret != C.TILEDB_OK {
		return fmt.Errorf("Error submitting query: %s", q.context.LastError())
	}

	return nil
}

/*
SubmitAsync a TileDB query

Async does not currently support the callback function parameter
To monitor progress of a query in a non blocking manner the status can be
polled:

 // Start goroutine for background monitoring
 go func(query Query) {
  var status QueryStatus
  var err error
   for status, err = query.Status(); status == TILEDB_INPROGRESS && err == nil; status, err = query.Status() {
     // Do something while query is running
   }
   // Do something when query is finished
 }(query)
*/
func (q *Query) SubmitAsync() error {
	ret := C.tiledb_query_submit_async(q.context.tiledbContext, q.tiledbQuery, nil, nil)
	if ret != C.TILEDB_OK {
		return fmt.Errorf("Error submitting query: %s", q.context.LastError())
	}
	return nil
}

// Status returns the status of a query
func (q *Query) Status() (QueryStatus, error) {
	var status C.tiledb_query_status_t
	ret := C.tiledb_query_get_status(q.context.tiledbContext, q.tiledbQuery, &status)
	if ret != C.TILEDB_OK {
		return -1, fmt.Errorf("Error getting query status: %s", q.context.LastError())
	}
	return QueryStatus(status), nil
}

// Type returns the query type
func (q *Query) Type() (QueryType, error) {
	var queryType C.tiledb_query_type_t
	ret := C.tiledb_query_get_type(q.context.tiledbContext, q.tiledbQuery, &queryType)
	if ret != C.TILEDB_OK {
		return -1, fmt.Errorf("Error getting query type: %s", q.context.LastError())
	}
	return QueryType(queryType), nil
}

// HasResults Returns true if the query has results
// Applicable only to read queries (it returns false for write queries)
func (q *Query) HasResults() (bool, error) {
	var hasResults C.int32_t
	ret := C.tiledb_query_has_results(q.context.tiledbContext, q.tiledbQuery, &hasResults)
	if ret != C.TILEDB_OK {
		return false, fmt.Errorf("Error checking if query has results: %s", q.context.LastError())
	}
	return int(hasResults) == 1, nil
}

// SetCoordinates sets the coordinate buffer
func (q *Query) SetCoordinates(coordinates interface{}) (*uint64, error) {
	return q.SetBuffer(TILEDB_COORDS, coordinates)
}
