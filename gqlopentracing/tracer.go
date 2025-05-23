package gqlopentracing

import (
	"context"
	"fmt"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"

	"github.com/99designs/gqlgen/graphql"
)

type (
	Tracer struct {
		DisableNonResolverBindingTrace bool
		OperationName                  string
	}
)

const (
	DefaultOperationName = "graphql"
)

var _ interface {
	graphql.HandlerExtension
	graphql.OperationInterceptor
	graphql.FieldInterceptor
} = Tracer{}

func (a Tracer) ExtensionName() string {
	return "OpenTracing"
}

func (a Tracer) Validate(schema graphql.ExecutableSchema) error {
	return nil
}

func (a Tracer) InterceptOperation(
	ctx context.Context,
	next graphql.OperationHandler,
) graphql.ResponseHandler {
	oc := graphql.GetOperationContext(ctx)

	// Get operation name
	operationName := a.OperationName
	if operationName == "" {
		operationName = DefaultOperationName
	}

	span, ctx := opentracing.StartSpanFromContext(ctx, operationName)
	ext.SpanKind.Set(span, "server")
	ext.Component.Set(span, "gqlgen")

	span.LogFields(
		log.String("raw-query", oc.RawQuery),
	)
	defer span.Finish()

	return next(ctx)
}

func (a Tracer) InterceptField(ctx context.Context, next graphql.Resolver) (any, error) {
	fc := graphql.GetFieldContext(ctx)

	// Check if this field is disabled
	if a.DisableNonResolverBindingTrace && !fc.IsMethod {
		return next(ctx)
	}

	span, ctx := opentracing.StartSpanFromContext(ctx, fc.Object+"_"+fc.Field.Name)
	span.SetTag("resolver.object", fc.Object)
	span.SetTag("resolver.field", fc.Field.Name)
	defer span.Finish()

	res, err := next(ctx)
	if err != nil {
		ext.Error.Set(span, true)
		span.LogFields(
			log.String("event", "error"),
			log.String("error.message", err.Error()),
			log.String("error.kind", fmt.Sprintf("%T", err)),
		)
	}

	errList := graphql.GetFieldErrors(ctx, fc)
	if len(errList) != 0 {
		ext.Error.Set(span, true)
		span.LogFields(
			log.String("event", "error"),
		)

		for idx, err := range errList {
			span.LogFields(
				log.String(fmt.Sprintf("error.%d.message", idx), err.Error()),
				log.String(fmt.Sprintf("error.%d.kind", idx), fmt.Sprintf("%T", err)),
			)
		}
	}

	return res, err
}
