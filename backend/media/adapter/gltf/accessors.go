package gltf

import (
	"context"
	"encoding/binary"
	"math"
)

func componentSize(kind int) int64 {
	switch kind {
	case 5120, 5121:
		return 1
	case 5122, 5123:
		return 2
	case 5125, 5126:
		return 4
	default:
		return 0
	}
}

func layout(kind string, size int64) ([]int64, int64) {
	columns, rows := int64(1), int64(0)
	switch kind {
	case "SCALAR":
		rows = 1
	case "VEC2":
		rows = 2
	case "VEC3":
		rows = 3
	case "VEC4":
		rows = 4
	case "MAT2":
		columns, rows = 2, 2
	case "MAT3":
		columns, rows = 3, 3
	case "MAT4":
		columns, rows = 4, 4
	}
	columnBytes := rows * size
	if columns > 1 {
		columnBytes = (columnBytes + 3) / 4 * 4
	}
	var offsets []int64
	for c := int64(0); c < columns; c++ {
		for r := int64(0); r < rows; r++ {
			offsets = append(offsets, c*columnBytes+r*size)
		}
	}
	return offsets, columnBytes * columns
}

func component(data []byte, kind int) float64 {
	switch kind {
	case 5120:
		return float64(int8(data[0]))
	case 5121:
		return float64(data[0])
	case 5122:
		return float64(int16(binary.LittleEndian.Uint16(data)))
	case 5123:
		return float64(binary.LittleEndian.Uint16(data))
	case 5125:
		return float64(binary.LittleEndian.Uint32(data))
	case 5126:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(data)))
	default:
		return math.NaN()
	}
}

func (d document) readAccessors(ctx context.Context, buffers [][]byte) ([][]float64, error) {
	result := make([][]float64, len(d.Accessors))
	var total int64
	for i, a := range d.Accessors {
		size := componentSize(a.ComponentType)
		offsets, elementBytes := layout(a.Type, size)
		if size == 0 || len(offsets) == 0 || a.Count < 1 || a.Count > maxDrawVertices || a.ByteOffset < 0 || a.ByteOffset%size != 0 || a.Normalized && (a.ComponentType == 5125 || a.ComponentType == 5126) {
			return nil, invalid("accessor shape")
		}
		total += a.Count * int64(len(offsets))
		if total > maxAccessorValues {
			return nil, invalid("decoded accessor budget")
		}
		values := make([]float64, a.Count*int64(len(offsets)))
		if a.BufferView != nil {
			view, err := d.viewBytes(buffers, *a.BufferView)
			if err != nil {
				return nil, err
			}
			v := d.BufferViews[*a.BufferView]
			stride := v.ByteStride
			if stride == 0 {
				stride = elementBytes
			}
			if stride < elementBytes || stride%size != 0 || (v.ByteOffset+a.ByteOffset)%size != 0 || a.ByteOffset > int64(len(view)) || (a.Count-1)*stride+elementBytes > int64(len(view))-a.ByteOffset {
				return nil, invalid("accessor byte bounds")
			}
			for n := int64(0); n < a.Count; n++ {
				if n%4096 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				for c, relative := range offsets {
					values[n*int64(len(offsets))+int64(c)] = component(view[a.ByteOffset+n*stride+relative:], a.ComponentType)
				}
			}
		} else if a.ByteOffset != 0 {
			return nil, invalid("accessor without view offset")
		}
		if a.Sparse != nil {
			s := a.Sparse
			indices, err := d.viewBytes(buffers, s.Indices.BufferView)
			if err != nil {
				return nil, err
			}
			data, err := d.viewBytes(buffers, s.Values.BufferView)
			if err != nil {
				return nil, err
			}
			indexBytes := componentSize(s.Indices.ComponentType)
			if s.Count < 1 || s.Count > a.Count || s.Indices.ComponentType != 5121 && s.Indices.ComponentType != 5123 && s.Indices.ComponentType != 5125 || s.Indices.ByteOffset < 0 || s.Values.ByteOffset < 0 || s.Indices.ByteOffset%indexBytes != 0 || s.Values.ByteOffset%size != 0 || s.Indices.ByteOffset > int64(len(indices)) || s.Values.ByteOffset > int64(len(data)) || s.Count*indexBytes > int64(len(indices))-s.Indices.ByteOffset || s.Count*elementBytes > int64(len(data))-s.Values.ByteOffset || d.BufferViews[s.Indices.BufferView].ByteStride != 0 || d.BufferViews[s.Values.BufferView].ByteStride != 0 || d.BufferViews[s.Indices.BufferView].Target != 0 || d.BufferViews[s.Values.BufferView].Target != 0 {
				return nil, invalid("sparse bounds")
			}
			previous := int64(-1)
			for n := int64(0); n < s.Count; n++ {
				position := int64(component(indices[s.Indices.ByteOffset+n*indexBytes:], s.Indices.ComponentType))
				if position <= previous || position >= a.Count {
					return nil, invalid("sparse index")
				}
				previous = position
				for c, relative := range offsets {
					values[position*int64(len(offsets))+int64(c)] = component(data[s.Values.ByteOffset+n*elementBytes+relative:], a.ComponentType)
				}
			}
		}
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > 1e12 {
				return nil, invalid("nonfinite binary value")
			}
		}
		if len(a.Min) != 0 && len(a.Min) != len(offsets) || len(a.Max) != 0 && len(a.Max) != len(offsets) {
			return nil, invalid("accessor min/max shape")
		}
		for c := range offsets {
			if len(a.Min) != 0 && len(a.Max) != 0 && a.Min[c] > a.Max[c] {
				return nil, invalid("accessor min/max order")
			}
			for n := int64(0); n < a.Count; n++ {
				value := values[n*int64(len(offsets))+int64(c)]
				if len(a.Min) != 0 && value < a.Min[c] || len(a.Max) != 0 && value > a.Max[c] {
					return nil, invalid("accessor min/max facts")
				}
			}
		}
		result[i] = values
	}
	return result, nil
}
