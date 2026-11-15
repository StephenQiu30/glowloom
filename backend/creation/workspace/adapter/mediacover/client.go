// Package mediacover consumes Media's generated closed-project cover facts.
package mediacover

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediacover"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Client has one explicit Media audience and no retry or alternate route.
type Client struct{address,token string;transport credentials.TransportCredentials}
var _ source.ProjectCoverReader=(*Client)(nil)
// New requires the dedicated Creation token and explicit private TLS policy.
func New(address,token string,transport credentials.TransportCredentials)(*Client,error){host,port,err:=net.SplitHostPort(address);number,parseErr:=strconv.Atoi(port);if err!=nil||host==""||strings.ContainsAny(host,"/@?#% ")||strings.IndexFunc(host,unicode.IsSpace)>=0||parseErr!=nil||number<1||number>65535||!serviceauth.ValidToken(token)||transport==nil||transport.Info().SecurityProtocol!="tls"{return nil,writefence.ErrInvalid};return &Client{address,token,transport.Clone()},nil}
// ReadProjectCover reads exactly once after the original project close barrier.
func(c *Client)ReadProjectCover(ctx context.Context,key source.ProjectCoverRequest)(source.ProjectCoverObservation,error){
	if c==nil{return source.ProjectCoverObservation{},writefence.ErrUnavailable};if err:=key.Validate();err!=nil{return source.ProjectCoverObservation{},err};if err:=ctx.Err();err!=nil{return source.ProjectCoverObservation{},err}
	bounded,cancel:=context.WithTimeout(ctx,10*time.Second);defer cancel();carrier:=propagation.MapCarrier{};propagation.TraceContext{}.Inject(bounded,carrier);values:=[]string{"authorization","Bearer "+c.token};for _,name:=range []string{"traceparent","tracestate"}{if value:=carrier[name];value!=""{values=append(values,name,value)}};bounded=metadata.NewOutgoingContext(bounded,metadata.Pairs(values...))
	connection,err:=grpc.NewClient(c.address,grpc.WithTransportCredentials(c.transport.Clone()),grpc.WithDisableRetry(),grpc.WithDisableServiceConfig(),grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4096),grpc.MaxCallSendMsgSize(4096),grpc.MaxRetryRPCBufferSize(0)));if err!=nil{return source.ProjectCoverObservation{},writefence.ErrUnavailable};defer func(){_=connection.Close()}()
	request:=&pb.ProjectCoverRequest{CloseId:key.CloseID[:],ProjectId:key.ProjectID[:],OrganizationId:key.OrganizationID[:],ScopeId:key.ScopeID[:],AssetId:key.AssetID[:],ProjectEpoch:key.ProjectEpoch,CoordinationId:key.CoordinationID[:]}
	response,err:=pb.NewProjectCoverStatesClient(connection).ReadProjectCover(bounded,request)
	if err!=nil{switch status.Code(err){case codes.Canceled:return source.ProjectCoverObservation{},context.Canceled;case codes.DeadlineExceeded:return source.ProjectCoverObservation{},context.DeadlineExceeded;default:return source.ProjectCoverObservation{},writefence.ErrUnavailable}}
	if response==nil||len(response.ProtoReflect().GetUnknown())!=0||response.Request==nil||len(response.Request.ProtoReflect().GetUnknown())!=0||!proto.Equal(request,response.Request)||response.AssetRevision<0||(response.Available&&response.AssetRevision<1){return source.ProjectCoverObservation{},writefence.ErrConflict}
	return source.ProjectCoverObservation{Request:key,Available:response.Available,AssetRevision:response.AssetRevision},nil
}
