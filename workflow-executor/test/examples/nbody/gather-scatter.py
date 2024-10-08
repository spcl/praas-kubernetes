from praassdk.msg import put, get, broadcast


def sync_results(worker_idx, n_workers, i, sub_data):
  # Write our partial results
  put(worker_idx, f"i:{i}", sub_data)

  # Get all other partial results
  all_data = list()
  for idx in range(n_workers):
    if idx == worker_idx:
      current_data = sub_data
    else:
      current_data = get(idx, f"i:{i}", target=idx, retain=True)
    all_data.append(current_data)
  return reduce(all_data)


def sync_results(worker_idx, n_workers, i, sub_data):
  msg_name = f"i:{i}"

  # Broadcast partial results
  others = list(range(n_workers))
  broadcast(others, msg_name, sub_data)

  # Get all other partial results
  all_data = list()
  for idx in range(n_workers):
    if idx == worker_idx:
      current_data = sub_data
    else:
      current_data = get(idx, msg_name)
    all_data.append(current_data)
  return reduce(all_data)
