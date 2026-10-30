@0xc5f3611bd0ea5432;

using Go = import "/go.capnp";
$Go.package("gen");
$Go.import("github.com/kikz0r/edgesync/spikes/capnp/gen");

struct ClipAnnounce {
  origin @0 :Data;
  seq @1 :UInt64;
  formats @2 :List(Format);

  struct Format {
    mime @0 :Text;
    size @1 :Int64;
  }
}
