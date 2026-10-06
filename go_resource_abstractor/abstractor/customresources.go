package abstractor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oakestra/oakestra/go_resource_abstractor/internal/errs"
	"github.com/oakestra/oakestra/go_resource_abstractor/internal/hooks"
	"github.com/oakestra/oakestra/go_resource_abstractor/internal/store"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

// CustomResources is the custom-resource sub-service: registering resource
// type definitions (each backed by its own dynamically named MongoDB
// collection) and create/read/update/delete for their instances. Pre/post
// webhooks fire under the resource_type itself as the entity name, so a
// hook registered for a given custom type only sees writes to that type's
// instances.
type CustomResources struct {
	store      *store.Store
	dispatcher *hooks.Hooks
}

func (c *CustomResources) entityFor(resourceType string) entityHooks {
	return entityHooks{dispatcher: c.dispatcher, name: resourceType}
}

// requireDefinition validates resourceType's shape and looks up its
// definition, returning errs.ErrNotFound if it isn't registered. Every
// instance operation needs both checks before touching the type's dynamic
// collection.
func (c *CustomResources) requireDefinition(ctx context.Context, resourceType string) (model.CustomResourceDefinition, error) {
	if err := store.ValidateResourceType(resourceType); err != nil {
		return model.CustomResourceDefinition{}, err
	}
	return c.store.FindCustomResourceByType(ctx, resourceType)
}

// ListDefinitions returns every registered custom resource type.
func (c *CustomResources) ListDefinitions(ctx context.Context) ([]model.CustomResourceDefinition, error) {
	return c.store.FindCustomResources(ctx)
}

// CreateDefinition registers a new custom resource type. The store rejects
// a resource_type that can't safely become a MongoDB collection name (see
// store.ValidateResourceType).
func (c *CustomResources) CreateDefinition(ctx context.Context, data model.CustomResourceDefinition) (model.CustomResourceDefinition, error) {
	return c.store.CreateCustomResource(ctx, data)
}

// DeleteDefinition removes a type definition and every instance of it,
// instances first so a failure on the second step can't orphan instances
// with no definition left to reach them. Returns the deleted definition and
// how many instances were removed alongside it.
func (c *CustomResources) DeleteDefinition(ctx context.Context, resourceType string) (model.CustomResourceDefinition, int64, error) {
	if _, err := c.requireDefinition(ctx, resourceType); err != nil {
		return model.CustomResourceDefinition{}, 0, err
	}

	deletedCount, err := c.store.DeleteAllResources(ctx, resourceType)
	if err != nil {
		return model.CustomResourceDefinition{}, 0, err
	}

	def, err := c.store.DeleteCustomResourceByType(ctx, resourceType)
	if err != nil && !errors.Is(err, errs.ErrNotFound) {
		return model.CustomResourceDefinition{}, deletedCount, err
	}
	return def, deletedCount, nil
}

// ListInstances returns instances of resourceType matching filter, a plain
// field-equality filter compared as strings. No filter key may start with
// "$": an unvalidated key reaching MongoDB verbatim would let a caller pass
// an operator like $where straight into the query.
func (c *CustomResources) ListInstances(ctx context.Context, resourceType string, filter map[string]string) ([]model.CustomResourceInstance, error) {
	if _, err := c.requireDefinition(ctx, resourceType); err != nil {
		return nil, err
	}
	if err := validateFilterKeys(filter); err != nil {
		return nil, err
	}

	mongoFilter := bson.M{}
	for k, v := range filter {
		mongoFilter[k] = v
	}
	return c.store.FindResources(ctx, resourceType, mongoFilter)
}

// GetInstance returns a single instance of resourceType by id.
func (c *CustomResources) GetInstance(ctx context.Context, resourceType, id string) (model.CustomResourceInstance, error) {
	if _, err := c.requireDefinition(ctx, resourceType); err != nil {
		return model.CustomResourceInstance{}, err
	}
	return c.store.FindResourceByID(ctx, resourceType, id)
}

// CreateInstance runs pre_create/post_create (under resourceType as the
// entity name) around inserting a new instance, after validating data
// against the type's stored JSON Schema.
func (c *CustomResources) CreateInstance(ctx context.Context, resourceType string, data model.CustomResourceInstance) (model.CustomResourceInstance, error) {
	if err := c.validateInstance(ctx, resourceType, data); err != nil {
		return model.CustomResourceInstance{}, err
	}

	entity := c.entityFor(resourceType)
	return create(ctx, entity, data, func(ctx context.Context, d model.CustomResourceInstance) (model.CustomResourceInstance, error) {
		return c.store.CreateResource(ctx, resourceType, d)
	})
}

// UpdateInstance runs pre_update/post_update around a $set patch, after
// re-validating data against the type's stored JSON Schema. Like other
// path-addressed updates, the pre_update hook sees data["_id"] = id.
func (c *CustomResources) UpdateInstance(ctx context.Context, resourceType, id string, patch model.CustomResourceInstance) (model.CustomResourceInstance, error) {
	if err := c.validateInstance(ctx, resourceType, patch); err != nil {
		return model.CustomResourceInstance{}, err
	}

	entity := c.entityFor(resourceType)
	return update(ctx, entity, id, patch, true, func(ctx context.Context, d model.CustomResourceInstance) (model.CustomResourceInstance, error) {
		return c.store.UpdateResource(ctx, resourceType, id, d)
	})
}

// DeleteInstance runs post_delete, keyed off id, around removing an
// instance. Deleting an id that was never created is not an error: it still
// fires post_delete regardless of whether anything was actually deleted.
func (c *CustomResources) DeleteInstance(ctx context.Context, resourceType, id string) error {
	if _, err := c.requireDefinition(ctx, resourceType); err != nil {
		return err
	}

	entity := c.entityFor(resourceType)
	_, err := del(ctx, entity, id, func(ctx context.Context) (model.CustomResourceInstance, error) {
		deleted, err := c.store.DeleteResource(ctx, resourceType, id)
		if errors.Is(err, errs.ErrNotFound) {
			return model.CustomResourceInstance{}, nil
		}
		return deleted, err
	})
	return err
}

// validateInstance checks that resourceType is registered and that v
// satisfies its stored JSON Schema.
func (c *CustomResources) validateInstance(ctx context.Context, resourceType string, v model.CustomResourceInstance) error {
	def, err := c.requireDefinition(ctx, resourceType)
	if err != nil {
		return err
	}
	payload, err := toMap(v)
	if err != nil {
		return err
	}
	return validateAgainstSchema(def, payload)
}

// validateFilterKeys rejects any filter key that looks like a MongoDB
// query operator.
func validateFilterKeys(filter map[string]string) error {
	for k := range filter {
		if strings.HasPrefix(k, "$") {
			return fmt.Errorf("%w: %q must not start with \"$\"", ErrInvalidFilterKey, k)
		}
	}
	return nil
}

// validateAgainstSchema validates data against def's stored JSON Schema. An
// absent or empty schema imposes no constraint.
//
// A schema that fails to compile comes back as a plain error, not
// ErrSchemaInvalid: rest maps it to an internal error rather than a 400,
// since a broken schema should fail closed, not accept anything against it.
func validateAgainstSchema(def model.CustomResourceDefinition, data map[string]any) error {
	if len(def.Schema) == 0 {
		return nil
	}

	body, err := json.Marshal(def.Schema)
	if err != nil {
		return fmt.Errorf("marshal stored schema: %w", err)
	}

	var parsedSchema jsonschema.Schema
	if err := json.Unmarshal(body, &parsedSchema); err != nil {
		return fmt.Errorf("decode stored schema: %w", err)
	}

	resolved, err := parsedSchema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve stored schema: %w", err)
	}

	if err := resolved.Validate(data); err != nil {
		return &schemaValidationError{msg: err.Error()}
	}
	return nil
}

// schemaValidationError keeps the jsonschema library's message intact while
// still satisfying errors.Is(err, ErrSchemaInvalid).
type schemaValidationError struct{ msg string }

func (e *schemaValidationError) Error() string        { return e.msg }
func (e *schemaValidationError) Is(target error) bool { return target == ErrSchemaInvalid }
