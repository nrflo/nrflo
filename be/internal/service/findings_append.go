package service

// AppendValue implements array-merge semantics for finding values:
//   - nil existing → return newValue as-is
//   - existing array + array new → flatten (merge arrays)
//   - existing array + scalar new → append element
//   - scalar existing + array new → [existing] + newArray
//   - scalar + scalar → [existing, new]
func AppendValue(existing, newValue interface{}) interface{} {
	if existing == nil {
		return newValue
	}

	existingArr, existingIsArr := existing.([]interface{})
	newArr, newIsArr := newValue.([]interface{})

	if existingIsArr {
		if newIsArr {
			return append(existingArr, newArr...)
		}
		return append(existingArr, newValue)
	}

	if newIsArr {
		return append([]interface{}{existing}, newArr...)
	}

	return []interface{}{existing, newValue}
}
