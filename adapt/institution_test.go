package adapt

import (
	"testing"

	"github.com/hyperjiang/futu/pb/qotcommon"
	"github.com/hyperjiang/futu/pb/qotgetinstitutionholdinglist"
	"github.com/hyperjiang/futu/pb/qotgetinstitutionlist"
	"github.com/hyperjiang/futu/protoid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestInstitutionProtocolIDs(t *testing.T) {
	should := require.New(t)
	should.Equal(uint32(3418), uint32(protoid.QotGetInstitutionList))
	should.Equal(uint32(3419), uint32(protoid.QotGetInstitutionProfile))
	should.Equal(uint32(3420), uint32(protoid.QotGetInstitutionDistribution))
	should.Equal(uint32(3421), uint32(protoid.QotGetInstitutionHoldingChange))
	should.Equal(uint32(3422), uint32(protoid.QotGetInstitutionHoldingList))
}

func TestInstitutionCursorOptionsAndHoldingWireContract(t *testing.T) {
	should := require.New(t)

	listOptions := NewOptions(
		With("page", "next-page"),
		With("count", 200),
		With("namePart", "Berkshire"),
	)
	listOptions["market"] = QotMarket_US
	var listRequest qotgetinstitutionlist.C2S
	should.NoError(listOptions.ToProto(&listRequest))
	should.Equal(QotMarket_US, listRequest.GetMarket())
	should.Equal("next-page", listRequest.GetPage())
	should.Equal(int32(200), listRequest.GetCount())
	should.Equal("Berkshire", listRequest.GetNamePart())

	holding := &qotgetinstitutionholdinglist.Response{
		RetType: proto.Int32(0),
		S2C: &qotgetinstitutionholdinglist.S2C{
			NextPage: proto.String("holding-next"),
			DataList: []*qotgetinstitutionholdinglist.HoldingListItem{{
				Security: &qotcommon.Security{Market: proto.Int32(QotMarket_US), Code: proto.String("AAPL")},
				Source:   proto.String("13F"),
			}},
		},
	}
	wire, err := proto.Marshal(holding)
	should.NoError(err)
	var decoded qotgetinstitutionholdinglist.Response
	should.NoError(proto.Unmarshal(wire, &decoded))
	should.Equal("holding-next", decoded.GetS2C().GetNextPage())
	should.Equal("AAPL", decoded.GetS2C().GetDataList()[0].GetSecurity().GetCode())
	should.Equal("13F", decoded.GetS2C().GetDataList()[0].GetSource())
}
