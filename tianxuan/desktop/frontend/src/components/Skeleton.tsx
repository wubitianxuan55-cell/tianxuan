export function Skeleton() {
  return (
    <div className="flex-1 px-8 py-10 flex flex-col gap-6 overflow-hidden">
      {/* User message */}
      <div className="flex flex-col gap-2.5 items-end pr-6">
        <div className="h-3 bg-border-soft rounded animate-[skeleton-pulse_1.5s_ease-in-out_infinite]" style={{ width: "60%" }} />
      </div>
      {/* Assistant reply */}
      <div className="flex flex-col gap-2.5 items-start">
        <div className="h-3 bg-border-soft rounded animate-[skeleton-pulse_1.5s_ease-in-out_infinite]" style={{ width: "90%" }} />
        <div className="h-3 bg-border-soft rounded animate-[skeleton-pulse_1.5s_ease-in-out_infinite]" style={{ width: "75%" }} />
        <div className="h-3 bg-border-soft rounded animate-[skeleton-pulse_1.5s_ease-in-out_infinite]" style={{ width: "45%" }} />
      </div>
      {/* Tool call */}
      <div className="flex py-2 px-3 ml-5 border border-border-soft rounded-md">
        <div className="h-3 bg-border-soft rounded animate-[skeleton-pulse_1.5s_ease-in-out_infinite]" style={{ width: "30%" }} />
      </div>
      {/* Another assistant reply */}
      <div className="flex flex-col gap-2.5 items-start">
        <div className="h-3 bg-border-soft rounded animate-[skeleton-pulse_1.5s_ease-in-out_infinite]" style={{ width: "80%" }} />
        <div className="h-3 bg-border-soft rounded animate-[skeleton-pulse_1.5s_ease-in-out_infinite]" style={{ width: "50%" }} />
      </div>
    </div>
  );
}
