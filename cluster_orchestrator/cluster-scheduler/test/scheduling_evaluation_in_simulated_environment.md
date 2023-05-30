# Scheduling Evaluation in Simulated Edge Environment

**Setup Time**: <5 minutes  
**Script Execution Time**:<left> heavily depends on number of `experiments`, `runs` per experiment, `number_of_nodes` in simulated edge network, `iterations` in Vivaldi Network Coordinate System, etc. Ranges from seconds to multiple hours.</left>

This experiment sets up a simulated edge environment that can then be used to evaluate the service scheduling algorithms.
Currently, there are two different scheduling algorithms, namely **R**esource **O**nly **Mapping** (ROM) and **L**atency 
and **D**istance aware **P**lacement (LDP).   
Within the test file `scheduling_simulated_environment.py` there are several possibilities to customize the simulated edge environment
with respect to the number of experiments to run, the number of runs per experiment that use the same environmental setup,
the number of worker nodes, and the number of update iterations within the Vivaldi Network Coordinate System (NCS), the number
of neighbors used during update phase of the Vivaldi NCS. Furthermore, it is possible to configure the *Service-to-User* 
and *Service-to-Service* latency and distance constraints.

## Step 1:
Check out the [service-scheduling](https://github.com/oakestra/oakestra/tree/service-scheduling) branch, open a console 
window inside it and navigate to the test directory of the cluster scheduler (`/src/cluster_orchestrator/cluster_scheduler/test`).
Once inside the folder, install the script requirements via:  
`pip install -r requirements.txt`

## Step 2:
Adapt the run configuration of the simulated edge environment in `scheduling_simulated_environment.py` as desired.   
The configurable parameter are as follows: 
- `experiments`: Number of experiments
- `runs`: Number of runs in each experimental setup
- `number_of_nodes`: Number of worker nodes in the simulated environment
- `vivaldi_coordinate_dimension`: Dimension of the Vivaldi Network Coordinates
- `iterations`: Number of update phases in the Vivaldi NCS
- `neighbors`: Number of neighbors used by each Vivaldi node during the update phase
- `s2u_geo_location`: Target location for *Service-to-User* scheduling with a geographic constraint
- `s2u_geo_threshold`: Threshold for the *Service-to-User* geographic constraint in kilometers
- `s2u_lat_area`: Geographic area for which the *Service-to-User* latency constraint should be fulfilled (currently available mappigns: *munich*, *garching*, *germany*)
- `s2u_lat_threshold`: Threshold for the *Service-to-User* latency constraint in milliseconds
- `s2s_target_id`: Target worker node id for *Service-to-Service* scheduling
- `s2s_lat_threshold`: Threshold for the *Service-to-Service* latency constraint in milliseconds
- `s2s_geo_threshold`: Threshold for the *Service-to-Service* geographic constraint in kilometers

To replicate the scheduling results of the simulated edge environment as seen in figure 10 of the paper, four tests were
performed with a varying number of worker nodes, i.e., `number_of_nodes=10,50,100,500`, and the following parameters, 
that were shared among the four tests:  
- `experiments=5000` 
- `runs=5` 
- `vivaldi_coordinate_dimension=2`
- `iterations=100` 
- `neighbors=6` 
- `s2u_geo_location= "48.13189654815318, 11.585990847392225"` 
- `s2u_geo_threshold=100` 
- `s2u_lat_area=”munich”` 
- `s2u_lat_threshold=20`
- `s2s_target_id="6"`
- `s2s_lat_threshold=20` 
- `s2s_geo_treshold=100`

## Step 3:
Run the test using the following command:  
`pytest scheduling_simulated_environment.py`  
Use the flag `-s` which is a shortcut for `--capture=no`, to allow to see the print statements in the console, that show the test 
progress and scheduling process.

## Step 4:
Once all desired tests were executed, the resutls can be found in `/src/cluster_orchestrator/cluster_scheduler/test/results`.
Copy the created result files and paste them in the following path of the artifacts bundle: `/Plots/evaluation2/results/test-scheduling/simulation`.
Open the jupyter notebook `Oakestra_Scheduling_Evaluation .ipynb` located in `/Plots/evaluation2/results` and the cells
to obtain the visualization of the scheduling results within a simulated edge environment.
