from executor.praas.scheduling.greedy import merge
from executor.praas.scheduling.round_robin import round_robin, maximal_use

job_schedulers = {
    "rr": round_robin,
    "spread": maximal_use,
    "merge": merge,
}

service_schedulers = {

}
