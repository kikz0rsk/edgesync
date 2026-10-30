@0x8518642835b94285;

using Go = import "/go.capnp";
$Go.package("gen");
$Go.import("github.com/kikz0r/edgesync/spikes/capnp/gen");

struct Hello {
  protocolVersion @0 :UInt16;
  buildVersion @1 :Text;
  role @2 :Role;
  name @3 :Text;
  peerId @4 :Data;
  macs @5 :List(Data);
  desktop @6 :Desktop;

  enum Role {
    client @0;
    server @1;
  }

  struct Desktop {
    monitors @0 :List(Monitor);

    struct Monitor {
      id @0 :Text;
      x @1 :Int32;
      y @2 :Int32;
      w @3 :Int32;
      h @4 :Int32;
      scalePermille @5 :UInt16;
    }
  }
}
