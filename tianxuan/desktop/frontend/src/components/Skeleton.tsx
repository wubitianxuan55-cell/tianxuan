export function Skeleton() {
  return (
    <div className="skeleton">
      {/* User message */}
      <div className="skeleton__msg skeleton__msg--user">
        <div className="skeleton__line" style={{ width: "60%" }} />
      </div>
      {/* Assistant reply */}
      <div className="skeleton__msg skeleton__msg--assistant">
        <div className="skeleton__line" style={{ width: "90%" }} />
        <div className="skeleton__line" style={{ width: "75%" }} />
        <div className="skeleton__line" style={{ width: "45%" }} />
      </div>
      {/* Tool call */}
      <div className="skeleton__tool">
        <div className="skeleton__line" style={{ width: "30%" }} />
      </div>
      {/* Another assistant reply */}
      <div className="skeleton__msg skeleton__msg--assistant">
        <div className="skeleton__line" style={{ width: "80%" }} />
        <div className="skeleton__line" style={{ width: "50%" }} />
      </div>
    </div>
  );
}
